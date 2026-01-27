# AgentTask Operator Architecture

## System Components

```mermaid
graph TD
    User[User / Client] -->|kubectl apply AgentTask| API[Kubernetes API Server]
    Controller[AgentTask Controller] -->|Watch AgentTask| API

    subgraph Cluster
        API
        Controller

        subgraph "Execution Plane"
            Pod[Hardened Pod (Default)]
            Sandbox[Agent Sandbox (Optional)]
        end

        Controller -->|Create/Manage| Pod
        Controller -->|Create/Manage| Sandbox

        Pod -->|Write Result| Volume[Scratch Volume]
        Sandbox -->|Write Result| Volume
    end
```

## Data Flow

1.  **Submission**: User submits `AgentTask` CR via `kubectl` or `agentctl`.
2.  **Reconciliation**:
    - Controller detects new `Pending` task.
    - Validates request (policies, quotas).
    - Selects backend (Pod or Sandbox).
    - Creates the execution resource (e.g., a Pod with `securityContext` and `NetworkPolicy`).
3.  **Execution**:
    - Pod starts. Container entrypoint runs the user provided code.
    - Code writes output to `stdout` (logs) and artifacts to `/sandbox/output`.
    - Sidecar or wrapper script saves exit code and structured result (`result.json`).
4.  **Completion**:
    - Controller sees Pod completion.
    - Controller reads termination message or logs to determine success/failure.
    - Controller updates `AgentTask.Status`.
    - (Future) Artifacts are offloaded to object storage if configured.
5.  **Cleanup**:
    - TTL timer starts.
    - Resource is deleted after TTL.

## CRD State Machine

- **Pending**: Accepted by API, satisfying validation.
- **Scheduled**: Backend resource created (Pod/Sandbox).
- **Running**: Pod is executing (image pulled, container started).
- **Succeeded**: Code exited 0.
- **Failed**: Code exited non-zero, OOM, or internal error.
- **Timeout**: Wall-clock limit exceeded.
- **Canceled**: User requested cancellation.
