package handler

import (
	"net/http"

	"skci/notification/internal/common"
	"skci/notification/internal/provider"
)

// channelStatusRoles may see which external channels are configured: the
// school admins who own the templates and humas who runs campaigns.
var channelStatusRoles = map[string]bool{roleAdmin: true, roleSuperadmin: true, "humas": true}

// ChannelStatus is GET /notification-channels/status. It never returns a
// credential, only whether email/WhatsApp/push/WA campaigns can deliver, so
// the UI can show "belum dikonfigurasi" instead of a failing button.
func (h *Handler) ChannelStatus(w http.ResponseWriter, r *http.Request) {
	if !channelStatusRoles[common.FromCtx(r).ActiveRole] {
		common.WriteError(w, http.StatusForbidden, "Hanya admin atau humas yang dapat melihat status kanal notifikasi")
		return
	}
	common.WriteJSON(w, http.StatusOK, h.channelStates(), nil)
}

func (h *Handler) channelStates() []provider.ChannelState {
	forChannel := func(ch string) string {
		if rt, ok := h.Provider.(provider.ChannelRouter); ok {
			if p := rt.For(ch); p != nil {
				return p.Name()
			}
			return ""
		}
		if h.Provider == nil {
			return ""
		}
		return h.Provider.Name()
	}
	push := provider.DescribeChannel("push", "unconfigured")
	if h.PushProvider != nil && h.PushProvider.IsConfigured() {
		push = provider.DescribeChannel("push", "webpush")
	}
	campaignWA := ""
	if h.CampaignWhatsApp != nil {
		campaignWA = h.CampaignWhatsApp.Name()
	}
	return []provider.ChannelState{
		provider.DescribeChannel("email", forChannel("email")),
		provider.DescribeChannel("whatsapp", forChannel("whatsapp")),
		push,
		provider.DescribeChannel("whatsapp_campaign", campaignWA),
	}
}
