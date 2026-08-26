package ui

import (
	"fmt"

	"github.com/gofiber/fiber/v3"
	"github.com/pion/webrtc/v4"
	"github.com/theapemachine/errnie"
)

type fluidOffer struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

/*
registerFluidWebRTC adds non-trickle HTTP signaling. The field and particle
payloads themselves travel only over the resulting WebRTC data channels.
*/
func (hub *Hub) registerFluidWebRTC() {
	hub.app.Options("/webrtc/manifold", func(ctx fiber.Ctx) error {
		setFluidCORS(ctx)
		return ctx.SendStatus(fiber.StatusNoContent)
	})
	hub.app.Post("/webrtc/manifold", func(ctx fiber.Ctx) error {
		setFluidCORS(ctx)
		var request fluidOffer

		if err := ctx.Bind().Body(&request); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid fluid WebRTC offer")
		}

		if request.Type != webrtc.SDPTypeOffer.String() || request.SDP == "" {
			return fiber.NewError(fiber.StatusBadRequest, "fluid WebRTC offer is incomplete")
		}

		answer, err := hub.fluid.Answer(webrtc.SessionDescription{
			Type: webrtc.SDPTypeOffer,
			SDP:  request.SDP,
		})

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"failed to answer fluid WebRTC offer",
				err,
			))
			return fiber.NewError(
				fiber.StatusInternalServerError,
				fmt.Sprintf("failed to answer fluid WebRTC offer: %v", err),
			)
		}

		return ctx.JSON(fluidOffer{Type: answer.Type.String(), SDP: answer.SDP})
	})

	hub.app.Options("/diagnostics", func(ctx fiber.Ctx) error {
		setFluidCORS(ctx)
		return ctx.SendStatus(fiber.StatusNoContent)
	})
	hub.app.Get("/diagnostics", func(ctx fiber.Ctx) error {
		setFluidCORS(ctx)

		return ctx.JSON(diagnosticsToggleRequest{
			Enabled: hub.diagnostics != nil && hub.diagnostics.DiagnosticsEnabled(),
		})
	})
	hub.app.Post("/diagnostics", func(ctx fiber.Ctx) error {
		setFluidCORS(ctx)
		var request diagnosticsToggleRequest

		if err := ctx.Bind().Body(&request); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid diagnostics toggle")
		}

		if hub.diagnostics == nil {
			return fiber.NewError(
				fiber.StatusServiceUnavailable,
				"diagnostics collector is unavailable",
			)
		}

		hub.diagnostics.SetDiagnosticsEnabled(request.Enabled)

		return ctx.JSON(diagnosticsToggleRequest{Enabled: request.Enabled})
	})
}

func setFluidCORS(ctx fiber.Ctx) {
	ctx.Set(fiber.HeaderAccessControlAllowOrigin, "*")
	ctx.Set(fiber.HeaderAccessControlAllowHeaders, fiber.HeaderContentType)
	ctx.Set(fiber.HeaderAccessControlAllowMethods, fiber.MethodPost+", "+fiber.MethodOptions)
}
