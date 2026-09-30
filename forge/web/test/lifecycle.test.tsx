import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { StateMachine } from "@/components/lifecycle/state-machine";
import { ActivityTimeline } from "@/components/lifecycle/activity-timeline";
import {
  BACKUP_MACHINE, COMPOSE_MACHINE, DATABASE_MACHINE, DEPLOYMENT_MACHINE, POWER_MACHINE, stepStatuses,
} from "@/components/lifecycle/lifecycle-machines";
import { renderWithQuery } from "@/test/render";

describe("stepStatuses", () => {
  it("marks the pipeline complete when power is offline", () => {
    expect(stepStatuses(POWER_MACHINE, "offline")).toEqual(["done", "done", "done", "done"]);
  });

  it("marks the running step active for a running server", () => {
    expect(stepStatuses(POWER_MACHINE, "running")).toEqual(["done", "done", "active", "pending"]);
  });

  it("flags the failing step for a failed backup", () => {
    expect(stepStatuses(BACKUP_MACHINE, "failed")).toEqual(["done", "error", "pending", "pending"]);
  });

  it("skips the whole pipeline for a cancelled backup", () => {
    expect(stepStatuses(BACKUP_MACHINE, "cancelled")).toEqual(["skipped", "skipped", "skipped", "skipped"]);
  });

  it("resolves the deployment rollback path", () => {
    expect(stepStatuses(DEPLOYMENT_MACHINE, "rolling_back")).toEqual(["done", "done", "done", "done", "active", "pending"]);
    expect(stepStatuses(DEPLOYMENT_MACHINE, "rolled_back")).toEqual(["done", "done", "error", "pending", "pending", "pending"]);
    expect(stepStatuses(DEPLOYMENT_MACHINE, "completed")).toEqual(["done", "done", "done", "done", "done", "done"]);
  });

  it("completes compose pipelines at running and exited", () => {
    expect(stepStatuses(COMPOSE_MACHINE, "running")).toEqual(["done", "done", "done", "pending", "pending"]);
    expect(stepStatuses(COMPOSE_MACHINE, "exited")).toEqual(["done", "done", "done", "done", "done"]);
  });

  it("maps database provisioning states", () => {
    expect(stepStatuses(DATABASE_MACHINE, "queued")).toEqual(["active", "pending", "pending"]);
    expect(stepStatuses(DATABASE_MACHINE, "running")).toEqual(["done", "done", "done"]);
    expect(stepStatuses(DATABASE_MACHINE, "failed")).toEqual(["done", "error", "pending"]);
  });
});

describe("state machine", () => {
  it("renders step labels and the current state label", () => {
    renderWithQuery(<StateMachine machine={POWER_MACHINE} state="running" />);
    expect(screen.getByText("Offline")).toBeInTheDocument();
    expect(screen.getByText("Starting")).toBeInTheDocument();
    expect(screen.getAllByText("Running").length).toBeGreaterThanOrEqual(2);
    expect(screen.getByText("Stopping")).toBeInTheDocument();
  });

  it("renders a failed deployment with the rolled-back label", () => {
    renderWithQuery(<StateMachine machine={DEPLOYMENT_MACHINE} state="rolled_back" />);
    expect(screen.getByText("Rolled back")).toBeInTheDocument();
    expect(screen.getByText("In progress")).toBeInTheDocument();
  });

  it("announces the pipeline to assistive tech", () => {
    renderWithQuery(<StateMachine machine={BACKUP_MACHINE} state="archiving files" />);
    expect(screen.getByRole("list", { name: "Backup pipeline" })).toBeInTheDocument();
  });
});

describe("activity timeline", () => {
  const events = [
    { id: "e1", action: "server.power.stop", createdAt: "2026-08-16T10:00:00Z", actorEmail: "ops@example.com" },
    { id: "e2", action: "server:file.delete", createdAt: "2026-08-16T09:00:00Z" },
    { id: "e3", action: "server.deploy.promote", createdAt: "2026-08-16T08:00:00Z" },
  ];

  it("renders events with actors and relative times", () => {
    renderWithQuery(<ActivityTimeline events={events} />);
    expect(screen.getByText("server.power.stop")).toBeInTheDocument();
    expect(screen.getByText("by ops@example.com")).toBeInTheDocument();
    expect(screen.getAllByText(/ago/).length).toBeGreaterThanOrEqual(1);
  });

  it("shows the empty state", () => {
    renderWithQuery(<ActivityTimeline events={[]} />);
    expect(screen.getByText("No lifecycle events recorded yet.")).toBeInTheDocument();
  });
});