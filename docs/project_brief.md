# AgentTask Operator - Project Brief

## Overview

**AgentTask Operator** is a Kubernetes-native system designed to provide secure, ephemeral code execution for LLM agents. It runs untrusted Python/R code in isolated runtimes with strict resource limits, default-deny networking, and structured results.

**Goal**: Provide a repeatable, secure contract for executing untrusted code in production environments.

## Why this exists

LLM "code interpreter" tools are powerful but dangerous. AgentTask Operator solves the need for:

- **Strong Isolation**: Network deny, non-root, read-only rootfs.
- **Explicit Limits**: CPU, memory, and wall-clock timeouts.
- **Deterministic Lifecycle**: Idempotent controller with TTL cleanup.
- **Operability**: Clear status phases, metrics, and structured logs.

## Core Features

1.  **Security-First Execution**:
    - Default-deny networking (NetworkPolicy).
    - Non-root, read-only root filesystem.
    - No privilege escalation or hostPath mounts.
2.  **Execution Contract**:
    - `AgentTask` CRD defines the request.
    - `Status` provides deterministic phases (Pending, Scheduled, Running, Succeeded/Failed).
    - Log streaming and artifact retrieval via CLI.
3.  **Flexible Backends**:
    - **Hardened PodJob (Default)**: Portable to any K8s cluster.
    - **Agent Sandbox (Optional)**: Integration with `kubernetes-sigs/agent-sandbox` for stronger isolation.
4.  **Results & Artifacts**:
    - JSON results surfaced in status.
    - Large artifacts persisted via PVC (roadmap: pluggable sinks).
    - Stdout/stderr streaming.

## Architecture Highlights

- **Controller**: Reconciles `AgentTask` resources.
- **CRD**: `AgentTask` (v1alpha1).
- **CLI**: `agentctl` for easy interaction.
- **Observability**: Prometheus metrics, Kubernetes Events.

## Threat Model (Summary)

- **In Scope**:
  - Prevent accidental data exfiltration (NetworkPolicy).
  - Prevent host/cluster escape (PodSecurity, restricted mounts).
  - Limit resource abuse (Quotas, Limits).
- **Out of Scope**:
  - Full VM boundary isolation (unless using specific Sandbox backends).
  - Security of network allowlists (if user explicitly enables them).

## Design Principles

- **Secure by Default**: No insecure configurations out of the box.
- **Deterministic**: Predictable state machine and reconciliation.
- **Portable**: Works on Kind, Minikube, GKE, EKS, AKS.
- **Operable**: First-class metrics and logs.

## Roadmap

- **MVP**: CRD, Hardened Pod Backend, Log Streaming, Basic Metrics.
- **V1**: Artifacts, TTL Cleanup, Policy Validation, Sandbox Backend.
- **V2**: Multi-tenancy, Concurrency Queues, RuntimeClass support.
