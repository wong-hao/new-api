package service

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

const (
	BillingSourceWallet       = "wallet"
	BillingSourceSubscription = "subscription"
)

// PreConsumeBilling 根据用户计费偏好创建 BillingSession 并执行预扣费。
// 会话存储在 relayInfo.Billing 上，供后续 Settle / Refund 使用。
// 会话已存在时（任务重试换到另一个渠道）把预扣补到 preConsumedQuota，
// 和首次预扣一样必须由余额覆盖。
//
// 这是预扣费的唯一入口，所以「余额不足导致扣费失败」的提醒也挂在它的出口上：
// 每个出口都由 defer 检查一次，只在请求最终被拒时提醒，中间失败（例如
// wallet_first 回退订阅后成功）不会误报。
func PreConsumeBilling(c *gin.Context, preConsumedQuota int, relayInfo *relaycommon.RelayInfo) (apiErr *types.NewAPIError) {
	defer func() {
		notifyEmptyWalletRejection(apiErr, relayInfo)
	}()

	if relayInfo != nil && relayInfo.QuotaClamp != nil {
		return types.NewErrorWithStatusCode(
			relayInfo.QuotaClamp,
			types.ErrorCodeModelPriceError,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
	if preConsumedQuota < 0 {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("pre-consume quota cannot be negative: %d", preConsumedQuota),
			types.ErrorCodeModelPriceError,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
	if relayInfo != nil && relayInfo.Billing != nil {
		if err := relayInfo.Billing.ReserveBeforeSubmit(preConsumedQuota); err != nil {
			var apiErr *types.NewAPIError
			if errors.As(err, &apiErr) {
				return apiErr
			}
			return types.NewErrorWithStatusCode(err, types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry())
		}
		return nil
	}
	session, apiErr := NewBillingSession(c, relayInfo, preConsumedQuota)
	if apiErr != nil {
		return apiErr
	}
	relayInfo.Billing = session
	return nil
}

// notifyEmptyWalletRejection reminds a user whose balance is 0 that their request was
// rejected for lack of balance. A rejected request is the one case the low-quota
// reminder in quota.go cannot cover: that one fires after a charge has already
// succeeded, so a user with no balance to charge never hears from it.
//
// The reminder goes out through the recipient's own notification method, and it is not
// gated on the low-balance switches: those ask for at most one notice when a balance
// crosses a threshold, while this one reports a request that already failed, so a user
// who chose the message center has to receive it there. Its own notification type gives
// it its own limit budget, so a low-balance reminder cannot use up the one that
// reaches the user whose key has stopped working. It runs in the background and never
// affects the billing result or the error the client receives.
func notifyEmptyWalletRejection(apiErr *types.NewAPIError, relayInfo *relaycommon.RelayInfo) {
	if apiErr == nil || relayInfo == nil || relayInfo.IsPlayground || relayInfo.UserId == 0 {
		return
	}
	if apiErr.GetErrorCode() != types.ErrorCodeInsufficientUserQuota {
		return
	}

	gopool.Go(func() {
		balance, err := model.GetUserQuota(relayInfo.UserId, false)
		if err != nil {
			common.SysError(common.LogText("failed to read the quota of user %d for the empty-balance notice: %s", relayInfo.UserId, err.Error()))
			return
		}
		if balance > 0 {
			return
		}

		lang := relayInfo.UserSetting.Language
		title := i18n.Translate(lang, i18n.MsgNotifyQuotaEmptyTitle)
		content, values := quotaEmptyNotifyContent(lang, relayInfo.UserSetting.NotifyType, relayInfo.OriginModelName, PaymentReturnURL("/wallet"))
		if err := NotifyUser(relayInfo.UserId, relayInfo.UserEmail, relayInfo.UserSetting, dto.NewNotify(dto.NotifyTypeQuotaEmpty, title, content, values)); err != nil {
			common.SysError(common.LogText("failed to send the empty-balance notice to user %d: %s", relayInfo.UserId, err.Error()))
		}
	})
}

// ---------------------------------------------------------------------------
// SettleBilling — 后结算辅助函数
// ---------------------------------------------------------------------------

// SettleBilling 执行计费结算。如果 RelayInfo 上有 BillingSession 则通过 session 结算，
// 否则回退到旧的 PostConsumeQuota 路径（兼容按次计费等场景）。
func SettleBilling(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, actualQuota int) error {
	if relayInfo.Billing != nil {
		preConsumed := relayInfo.Billing.GetPreConsumedQuota()
		delta := actualQuota - preConsumed

		if delta > 0 {
			logger.LogInfo(ctx, common.LogText("charging the difference after pre-consume: %s (actual %s, pre-consumed %s)",
				logger.FormatQuota(delta),
				logger.FormatQuota(actualQuota),
				logger.FormatQuota(preConsumed),
			))
		} else if delta < 0 {
			logger.LogInfo(ctx, common.LogText("refunding the difference after pre-consume: %s (actual %s, pre-consumed %s)",
				logger.FormatQuota(-delta),
				logger.FormatQuota(actualQuota),
				logger.FormatQuota(preConsumed),
			))
		} else {
			logger.LogInfo(ctx, common.LogText("pre-consumed quota matches the actual quota, no adjustment: %s (per-request billing)",
				logger.FormatQuota(actualQuota),
			))
		}

		if err := relayInfo.Billing.Settle(actualQuota); err != nil {
			return err
		}

		// 发送额度通知（订阅计费使用订阅剩余额度）
		if actualQuota != 0 {
			if relayInfo.BillingSource == BillingSourceSubscription {
				checkAndSendSubscriptionQuotaNotify(relayInfo)
			} else {
				checkAndSendQuotaNotify(relayInfo, actualQuota-preConsumed, preConsumed)
			}
		}
		return nil
	}

	// 回退：无 BillingSession 时使用旧路径
	quotaDelta := actualQuota - relayInfo.FinalPreConsumedQuota
	if quotaDelta != 0 {
		return PostConsumeQuota(relayInfo, quotaDelta, relayInfo.FinalPreConsumedQuota, true)
	}
	return nil
}
