# A Microkernel Runtime for AI Agents

## Executive Summary

This document proposes an architectural approach for building a next-generation agent runtime. Rather than creating another personal assistant, coding agent, or agent framework, the goal is to build a **small, stable operating environment** capable of hosting arbitrary AI agents.

The guiding philosophy is simple:

> **The runtime should own infrastructure. Agents should own behavior.**

Instead of shipping a monolithic system with opinions about memory, RAG, routing, planning, or UI, the runtime provides only the infrastructure required to execute agents safely and reliably. Everything else is implemented as replaceable plugins or installable capability packages.

This approach borrows heavily from operating system design, particularly microkernel architectures.

---

# Motivation

Most current agent frameworks have evolved around a specific use case:

* coding assistants
* personal assistants
* research agents
* browser automation
* desktop automation

As these systems mature, they are increasingly converging toward the same underlying architecture:

* model interaction
* tool calling
* permissions
* state management
* event logging
* checkpointing
* plugins

The distinction between a "coding agent" and a "general agent" is becoming less about architecture and more about configuration.

Rather than creating another specialized framework, this proposal attempts to identify the common runtime beneath all of them.

---

# Design Philosophy

## Runtime owns infrastructure.

The runtime is responsible for:

* execution
* state
* identity
* permissions
* plugin lifecycle
* event processing
* artifact tracking
* capability enforcement

The runtime is **not** responsible for:

* memory implementation
* context strategy
* planning algorithms
* routing logic
* RAG
* browser automation
* desktop automation
* UI
* assistant personality

Those become replaceable system components.

---

# Core Principle

The guiding rule throughout the design is:

> **The runtime owns execution. Plugins own policy.**

Examples:

The runtime determines **when** context is requested.

The Context Plugin determines **how** context is assembled.

The runtime determines **when** memory is persisted.

The Memory Plugin determines **what** should be remembered.

The runtime determines **when** permissions are evaluated.

The Permission Provider determines **whether** an action is allowed.

---

# Why a Microkernel?

Traditional frameworks tend to accumulate features over time.

The result is:

* tightly coupled components
* difficult customization
* difficult testing
* difficult replacement
* architectural bloat

Instead, the runtime should resemble a microkernel.

The kernel remains intentionally small and stable while optional functionality lives outside the kernel.

---

# High-Level Architecture

```
                Clients

      CLI
      Desktop UI
      Web UI
      REST API
      IDE Extensions

                │

        Runtime Host (Daemon)

        lifecycle
        communication
        scheduling
        local API
        plugin processes

                │

        Runtime Kernel

        execution
        permissions
        state
        events
        artifacts
        plugin interfaces
        agent lifecycle

                │

             Plugins

        memory
        context
        routing
        tools
        model adapters
        evaluators
        UI extensions
```

---

# Runtime Kernel

The Runtime Kernel should be deliberately boring.

Responsibilities include:

* execution loop
* state management
* checkpointing
* permission enforcement
* event bus
* artifact tracking
* plugin loading
* agent lifecycle
* capability enforcement

Everything should be deterministic and replayable.

---

# Runtime Host

The Runtime Host is separate from the Runtime Kernel.

Responsibilities:

* daemon lifecycle
* communication listeners
* scheduler
* plugin process supervision
* local API
* health monitoring
* updates

The host is analogous to systemd or launchd.

The kernel remains usable without the daemon for development.

---

# Development Strategy

Rather than beginning with the daemon, development proceeds in stages.

First:

```
Runtime Kernel

↓

CLI Development Host
```

Later:

```
Runtime Kernel

↓

Daemon Host

↓

UI Clients
```

The CLI proves the kernel.

The daemon hosts the kernel.

Neither contains the business logic.

---

# Domain Model

The runtime should define stable core entities from the beginning.

```
Owner

Workspace

Profile

Agent

Run

Step

Artifact

Capability

Principal

Policy

Plugin
```

These become the vocabulary of the system.

---

# Owner-Centric Architecture

Unlike enterprise orchestration systems, this runtime is designed primarily for a single owner.

```
Owner

↓

Multiple Workspaces

↓

Multiple Profiles

↓

Multiple Agents
```

The owner may maintain different operating profiles.

Examples:

* Personal
* Work
* Finance
* Development
* Research

Each profile may expose different:

* credentials
* permissions
* models
* memory
* tools
* files

---

# Agents

An agent is not part of the kernel.

Instead, the runtime hosts agents.

The runtime knows:

* how to start an agent
* how to stop an agent
* how to assign permissions
* how to track execution
* how to record events

The runtime does **not** know:

* how the agent plans
* how it remembers
* how it builds context
* how it routes work

These become configurable services.

---

# Capabilities

Capabilities are installable packages.

Examples:

* Google Workspace
* Filesystem
* Git
* Browser
* Excel
* OCR
* PDF
* Slack

A capability package may provide:

* tools
* schemas
* UI extensions
* artifact viewers
* documentation
* tests

Capabilities request permissions.

The runtime grants permissions.

---

# Identity

Identity is a core abstraction.

The runtime distinguishes between:

* Owner
* Agent
* Tool
* Service

Credential storage is plugin-based.

Identity enforcement is kernel-based.

This separation allows multiple credential providers without changing runtime behavior.

---

# Permissions

Permissions are enforced centrally.

Every capability requests scopes.

Every action is evaluated against policy.

Possible actions include:

* allow
* ask
* deny

Permissions become composable across:

* owner
* workspace
* profile
* capability
* agent
* run

---

# Events

Everything emits events.

Examples:

* run started
* tool called
* permission requested
* artifact created
* checkpoint saved
* run completed

The event stream becomes the backbone for:

* replay
* debugging
* evaluation
* UI
* auditing

---

# State

Runs should be resumable.

Checkpointing should occur after meaningful execution boundaries, particularly after tool execution.

The runtime should support:

* replay
* recovery
* deterministic debugging

---

# Artifacts

Agents produce artifacts.

Examples:

* reports
* spreadsheets
* images
* code patches
* logs
* documents

Artifacts become first-class runtime objects.

Each artifact records:

* creator
* producing step
* MIME type
* lineage
* storage location

---

# Plugins

Plugin categories include:

* model adapters
* context providers
* memory providers
* routing policies
* tool providers
* evaluators
* communication providers
* schedulers
* UI extensions

The runtime should only depend on interfaces.

---

# Multi-Agent Support

The runtime should be multi-agent aware from the beginning.

The kernel should understand:

* agent registry
* handoffs
* ownership
* capability boundaries

Routing strategies should remain plugins.

The runtime hosts multiple agents.

It does not prescribe how they collaborate.

---

# Communication

Communication should also be plugin-based.

Examples:

* Gmail
* Slack
* Discord
* Webhooks
* Local IPC
* File watchers

These plugins generate inbound events.

The runtime dispatches them.

---

# UI

The runtime should not include a built-in UI.

Instead, it exposes a stable local API.

Possible clients include:

* CLI
* Desktop
* Web
* Mobile
* IDE extensions

UI becomes another consumer of the runtime.

---

# Why This Is Not Another Agent Framework

Most frameworks combine:

* execution
* planning
* memory
* routing
* UI
* assistant behavior

This proposal intentionally separates those concerns.

The runtime should remain useful even if:

* memory implementations change
* models change
* MCP evolves
* planning techniques improve
* new routing algorithms appear

The kernel should remain largely unchanged.

---

# Long-Term Vision

Rather than thinking in terms of assistants or coding agents, this proposal views the runtime as an operating environment for AI.

A useful analogy is:

| Traditional Operating System | Agent Runtime              |
| ---------------------------- | -------------------------- |
| User                         | Owner                      |
| Process                      | Run                        |
| Thread                       | Step                       |
| Application                  | Agent                      |
| Driver                       | Capability                 |
| Filesystem                   | Resource Mounts            |
| System Calls                 | Tool Calls                 |
| Scheduler                    | Routing/Scheduling Plugins |
| Event Log                    | Runtime Event Stream       |
| Keychain                     | Credential Provider        |

Under this model:

* the runtime is infrastructure,
* agents are applications,
* capabilities are installed software,
* plugins are replaceable system services.

The LLM becomes an interchangeable execution engine rather than the defining characteristic of the platform.

---

# Open Questions

The following areas require further architectural discussion before implementation:

1. Should model adapters be plugins or kernel services?
2. How should plugin versioning and compatibility be negotiated?
3. Should communication plugins live in the Runtime Host or be independently supervised?
4. What is the ideal capability/package format?
5. How should resource mounts be represented?
6. Should the event bus be internal only or externally subscribable?
7. How should distributed execution fit into the architecture?
8. How should policy composition be resolved when multiple profiles conflict?
9. What guarantees should checkpointing provide?
10. What is the minimal stable kernel that can remain unchanged for years?

---

# Request for Feedback

The primary goal of this proposal is to validate the architectural direction before implementation.

Specific feedback is requested on:

* Kernel boundary
* Runtime Host responsibilities
* Plugin architecture
* Domain model
* Identity model
* Capability model
* Event-driven design
* Long-term extensibility
* Comparison to existing agent frameworks
* Potential architectural blind spots

The intent is to produce a runtime whose core remains small, stable, and extensible while allowing rapid evolution of agents, capabilities, and AI techniques without requiring continual redesign of the underlying system.
