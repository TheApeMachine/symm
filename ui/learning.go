package ui

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/strategy"
)

/* SetLearner exposes on-demand state and the durable forward-decision journal. */
func (hub *Hub) SetLearner(learner *strategy.Agent, runID hindsight.RunID) {
	hub.app.Get("/learning", func(request fiber.Ctx) error {
		// This is a UI request deadline, not a learning horizon or market gate.
		ctx, cancel := context.WithTimeout(hub.ctx, 5*time.Second)
		defer cancel()
		view, err := learner.Snapshot(ctx, request.Query("symbol"))
		if err != nil {
			return err
		}
		return request.JSON(view)
	})
	/*
		The skill endpoint answers the one question the whole terminal needs on
		every surface: how competent is the agent, and what is it allowed to do
		about it. It is deliberately a small projection of the same coherent
		snapshot, so the top bar can never disagree with the learning surface.
	*/
	hub.app.Get("/learning/skill", func(request fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(hub.ctx, 5*time.Second)
		defer cancel()
		view, err := learner.Snapshot(ctx, "")
		if err != nil {
			return err
		}
		return request.JSON(fiber.Map{
			"skill":      view.Skill,
			"dispatched": view.Dispatched,
			"decisions":  view.Decisions,
			"resolved":   view.Resolved,
			"symbols":    len(view.Universe),
		})
	})
	hub.app.Get("/learning/events", func(request fiber.Ctx) error {
		// One operator inspection page. Learning itself has no history limit.
		events, err := hub.store.LearningEvents(runID, request.Query("symbol"), 200)
		if err != nil {
			return err
		}
		return request.JSON(events)
	})
}
