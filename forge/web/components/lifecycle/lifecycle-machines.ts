"use client";

export type StepStatus = "done" | "active" | "pending" | "error" | "skipped";

export interface LifecycleStep {
  id: string;
  labelKey: string;
  fallback: string;
}

export interface LifecycleMachine {
  id: string;
  fallbackName: string;
  steps: LifecycleStep[];
  activeIndex: (state: string) => number;
  completedThrough: (state: string) => number;
  failedStates: string[];
  cancelledStates?: string[];
  label: (state: string) => string;
}

const DEF: Record<string, LifecycleStep> = {
  queued: { id: "queued", labelKey: "lifecycle.queued", fallback: "Queued" },
  starting: { id: "starting", labelKey: "lifecycle.starting", fallback: "Starting" },
  running: { id: "running", labelKey: "lifecycle.running", fallback: "Running" },
  stopping: { id: "stopping", labelKey: "lifecycle.stopping", fallback: "Stopping" },
  offline: { id: "offline", labelKey: "lifecycle.offline", fallback: "Offline" },
  creating: { id: "creating", labelKey: "lifecycle.creating", fallback: "Creating backup" },
  archiving: { id: "archiving", labelKey: "lifecycle.archiving", fallback: "Archiving files" },
  completed: { id: "completed", labelKey: "lifecycle.completed", fallback: "Completed" },
  pending: { id: "pending", labelKey: "lifecycle.pending", fallback: "Pending" },
  provisioning: { id: "provisioning", labelKey: "lifecycle.provisioning", fallback: "Provisioning" },
  inProgress: { id: "in-progress", labelKey: "lifecycle.in-progress", fallback: "In progress" },
  awaitingHealth: { id: "awaiting-health", labelKey: "lifecycle.awaiting-health", fallback: "Awaiting health" },
  promoting: { id: "promoting", labelKey: "lifecycle.promoting", fallback: "Promoting" },
  rollingBack: { id: "rolling-back", labelKey: "lifecycle.rolling-back", fallback: "Rolling back" },
  exited: { id: "exited", labelKey: "lifecycle.exited", fallback: "Exited" },
  paused: { id: "paused", labelKey: "lifecycle.paused", fallback: "Paused" },
};

export const POWER_MACHINE: LifecycleMachine = {
  id: "power",
  fallbackName: "Power state",
  steps: [DEF.offline, DEF.starting, DEF.running, DEF.stopping],
  activeIndex: (state) =>
    state === "starting" ? 1
    : state === "running" ? 2
    : state === "stopping" ? 3
    : 0,
  completedThrough: (state) => (state === "offline" ? 3 : -1),
  failedStates: [],
  label: (state) => DEF[state]?.fallback ?? state,
};

export const BACKUP_MACHINE: LifecycleMachine = {
  id: "backup",
  fallbackName: "Backup",
  steps: [DEF.queued, DEF.creating, DEF.archiving, DEF.completed],
  activeIndex: (state) =>
    state === "queued" ? 0
    : state === "running" || state === "creating backup" || state === "retrying" ? 1
    : state === "archiving files" ? 2
    : state === "completed" || state === "succeeded" ? 3
    : state === "failed" ? 1
    : -1,
  completedThrough: (state) => (state === "completed" || state === "succeeded" ? 3 : -1),
  failedStates: ["failed"],
  cancelledStates: ["cancelled"],
  label: (state) => (state === "creating backup" ? DEF.creating.fallback : DEF[state]?.fallback ?? state),
};

export const DEPLOYMENT_MACHINE: LifecycleMachine = {
  id: "deployment",
  fallbackName: "Deployment",
  steps: [DEF.pending, DEF.provisioning, DEF.inProgress, DEF.awaitingHealth, DEF.promoting, DEF.completed],
  activeIndex: (state) =>
    state === "pending" ? 0
    : state === "provisioning" ? 1
    : state === "in_progress" ? 2
    : state === "awaiting_health" ? 3
    : state === "promoting" ? 4
    : state === "rollback_pending" || state === "rolling_back" ? 4
    : state === "completed" ? 5
    : state === "failed" || state === "rolled_back" ? 2
    : -1,
  completedThrough: (state) => (state === "completed" ? 5 : -1),
  failedStates: ["failed", "rolled_back"],
  cancelledStates: ["cancelled"],
  label: (state) => {
    const mapped: Record<string, string> = {
      "in_progress": DEF.inProgress.fallback,
      "awaiting_health": DEF.awaitingHealth.fallback,
      "rollback_pending": DEF.rollingBack.fallback,
      "rolling_back": DEF.rollingBack.fallback,
      "rolled_back": "Rolled back",
    };
    return mapped[state] ?? DEF[state]?.fallback ?? state;
  },
};

export const COMPOSE_MACHINE: LifecycleMachine = {
  id: "compose",
  fallbackName: "Service state",
  steps: [DEF.offline, DEF.starting, DEF.running, DEF.stopping, DEF.exited],
  activeIndex: (state) =>
    state === "starting" || state === "restarting" ? 1
    : state === "running" || state === "paused" ? 2
    : state === "stopping" ? 3
    : state === "exited" || state === "dead" ? 4
    : 0,
  completedThrough: (state) => (state === "running" ? 2 : state === "exited" ? 4 : -1),
  failedStates: ["dead"],
  label: (state) => DEF[state]?.fallback ?? state,
};

export const DATABASE_MACHINE: LifecycleMachine = {
  id: "database",
  fallbackName: "Database provisioning",
  steps: [DEF.queued, DEF.provisioning, DEF.running],
  activeIndex: (state) =>
    state === "queued" ? 0
    : state === "provisioning" || state === "retrying" ? 1
    : state === "running" || state === "succeeded" ? 2
    : state === "failed" ? 1
    : -1,
  completedThrough: (state) => (state === "running" || state === "succeeded" ? 2 : -1),
  failedStates: ["failed"],
  cancelledStates: ["cancelled"],
  label: (state) => DEF[state]?.fallback ?? state,
};

export function stepStatuses(machine: LifecycleMachine, state: string): StepStatus[] {
  const active = machine.activeIndex(state);
  const completeThrough = machine.completedThrough(state);
  const failed = machine.failedStates.includes(state);
  const cancelled = machine.cancelledStates?.includes(state);
  return machine.steps.map((_, index) => {
    if (cancelled) return "skipped";
    if (failed && index === active) return "error";
    if (completeThrough >= 0 && index <= completeThrough) return "done";
    if (active >= 0 && index === active) return "active";
    if (active >= 0 && index < active) return "done";
    return "pending";
  });
}