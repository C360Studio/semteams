// Deployment API client
// Handles flow lifecycle operations (deploy, start, stop, undeploy)

const API_BASE = "/flowbuilder/deployment";

export type DeploymentOperation = "deploy" | "start" | "stop" | "undeploy";

export class DeploymentApiError extends Error {
  constructor(
    message: string,
    public operation: DeploymentOperation,
    public statusCode: number,
    public details?: unknown,
  ) {
    super(message);
    this.name = "DeploymentApiError";
  }
}

async function executeDeploymentOperation(
  flowId: string,
  operation: DeploymentOperation,
): Promise<void> {
  const response = await fetch(`${API_BASE}/${flowId}/${operation}`, {
    method: "POST",
  });

  if (!response.ok) {
    const error = await response.json().catch(() => ({}));
    throw new DeploymentApiError(
      `Failed to ${operation} flow: ${response.statusText}`,
      operation,
      response.status,
      error,
    );
  }
}

export const deploymentApi = {
  /**
   * Deploy a flow (write ComponentConfigs to KV)
   * Transitions: not_deployed → deployed_stopped
   */
  async deployFlow(flowId: string): Promise<void> {
    await executeDeploymentOperation(flowId, "deploy");
  },

  /**
   * Start a deployed flow (enable all components)
   * Transitions: deployed_stopped → running
   */
  async startFlow(flowId: string): Promise<void> {
    await executeDeploymentOperation(flowId, "start");
  },

  /**
   * Stop a running flow (disable all components)
   * Transitions: running → deployed_stopped
   */
  async stopFlow(flowId: string): Promise<void> {
    await executeDeploymentOperation(flowId, "stop");
  },

  /**
   * Undeploy a stopped flow (delete ComponentConfigs from KV)
   * Transitions: deployed_stopped → not_deployed
   */
  async undeployFlow(flowId: string): Promise<void> {
    await executeDeploymentOperation(flowId, "undeploy");
  },
};
