import { execSync } from "child_process";

/**
 * Checks if a port is available on the system.
 * @param port - Port number to check (1-65535)
 * @returns true if port is available, false if in use or on error
 * @throws Error if port number is invalid
 */
export function checkPortAvailability(port: number): boolean {
  // Validate port number - check NaN first
  if (Number.isNaN(port) || port < 1 || port > 65535) {
    throw new Error("Invalid port number");
  }
  if (!Number.isInteger(port)) {
    throw new Error("Port must be an integer");
  }

  try {
    const output = execSync(`lsof -i :${port}`, { encoding: "utf-8" });
    // Handle both string and Buffer returns (for testing compatibility)
    const outputStr = typeof output === "string" ? output : output.toString();
    // If output is non-empty, port is in use
    return outputStr.trim().length === 0;
  } catch {
    // lsof returns non-zero exit code when no process found OR on error
    // Conservative approach: return false (unavailable) on any error
    return false;
  }
}

/**
 * Finds the first available port in a given range.
 * @param startPort - Start of port range (inclusive)
 * @param endPort - End of port range (inclusive)
 * @returns First available port in range
 * @throws Error if no ports available or invalid range
 */
export function findAvailablePort(startPort: number, endPort: number): number {
  // Validate port range
  if (startPort < 1 || startPort > 65535 || Number.isNaN(startPort)) {
    throw new Error("Invalid port number");
  }
  if (endPort < 1 || endPort > 65535 || Number.isNaN(endPort)) {
    throw new Error("Invalid port number");
  }
  if (startPort > endPort) {
    throw new Error("Start port must be less than or equal to end port");
  }

  for (let port = startPort; port <= endPort; port++) {
    try {
      if (checkPortAvailability(port)) {
        return port;
      }
    } catch {
      // Skip ports that throw errors during checking and continue
      continue;
    }
  }

  throw new Error(`No available ports in range ${startPort}-${endPort}`);
}

/**
 * Detects running E2E containers by name pattern.
 * @param projectName - Optional project name to filter containers
 * @returns Array of container names matching E2E pattern
 */
export function detectE2EContainers(projectName?: string): string[] {
  try {
    const output = execSync('docker ps --format "{{.Names}}"', {
      encoding: "utf-8",
    });

    // Handle both string and Buffer returns (for testing compatibility)
    const outputStr = typeof output === "string" ? output : output.toString();

    if (!outputStr || outputStr.trim().length === 0) {
      return [];
    }

    const lines = outputStr.trim().split("\n");
    const containers: string[] = [];

    for (const line of lines) {
      const trimmedLine = line.trim();
      if (!trimmedLine) continue;

      // Handle both formats:
      // 1. Simple format (just names): "semstreams-ui-e2e-caddy"
      // 2. Table format (full docker ps output): extract last column (NAMES)
      let containerName = trimmedLine;

      // Check if this looks like a table header or table row
      if (
        trimmedLine.includes("CONTAINER ID") ||
        trimmedLine.includes("IMAGE")
      ) {
        // Skip header row
        continue;
      }

      // If line has multiple columns (spaces), extract the last column (NAMES)
      const columns = trimmedLine.split(/\s+/);
      if (columns.length > 1) {
        // Last column is the container name in table format
        containerName = columns[columns.length - 1];
      }

      // Base pattern: semstreams-ui-e2e-*
      const basePattern = /^semstreams-ui-e2e-/;

      if (!basePattern.test(containerName)) {
        continue;
      }

      // If project name is provided, filter by it
      if (projectName) {
        const projectPattern = new RegExp(`^semstreams-ui-e2e-${projectName}-`);
        if (projectPattern.test(containerName)) {
          containers.push(containerName);
        }
      } else {
        containers.push(containerName);
      }
    }

    return containers;
  } catch {
    // Docker not available or error executing command
    return [];
  }
}

/**
 * Generates a unique project name for E2E test isolation.
 * @param prefix - Optional prefix (defaults to 'e2e')
 * @returns Unique project name in format: prefix-timestamp-random
 */
export function getProjectName(prefix?: string): string {
  // Use default prefix if not provided or empty
  let sanitizedPrefix = prefix && prefix.trim().length > 0 ? prefix : "e2e";

  // Sanitize prefix: remove special characters, keep only alphanumeric and hyphens
  sanitizedPrefix = sanitizedPrefix.toLowerCase().replace(/[^a-z0-9-]/g, "");

  // Ensure we have a valid prefix after sanitization
  if (sanitizedPrefix.length === 0) {
    sanitizedPrefix = "e2e";
  }

  // Generate timestamp
  const timestamp = Date.now();

  // Generate random suffix (6 characters, alphanumeric lowercase)
  const randomSuffix = Math.random().toString(36).substring(2, 8);

  return `${sanitizedPrefix}-${timestamp}-${randomSuffix}`;
}

/**
 * Extracts the host port from caddy container in the list.
 * @param containers - Array of container names
 * @returns Port number if caddy container found with port mapping, null otherwise
 */
export function getPortFromContainers(containers: string[]): number | null {
  // Find caddy container
  const caddyContainer = containers.find((name) => name.includes("caddy"));

  if (!caddyContainer) {
    return null;
  }

  try {
    // Use docker inspect to get port mappings
    const output = execSync(`docker inspect ${caddyContainer}`, {
      encoding: "utf-8",
    });
    // Handle both string and Buffer returns (for testing compatibility)
    const outputStr = typeof output === "string" ? output : output.toString();
    const inspectData = JSON.parse(outputStr);

    if (!inspectData || inspectData.length === 0) {
      return null;
    }

    const networkSettings = inspectData[0]?.NetworkSettings;
    if (!networkSettings || !networkSettings.Ports) {
      return null;
    }

    // Look for 3000/tcp mapping (internal container port)
    const portMapping = networkSettings.Ports["3000/tcp"];
    if (!portMapping || portMapping.length === 0) {
      return null;
    }

    const hostPort = portMapping[0]?.HostPort;
    if (!hostPort) {
      return null;
    }

    return parseInt(hostPort, 10);
  } catch {
    // Container not found or inspect failed
    return null;
  }
}

/**
 * Selects an available port for E2E tests using the following strategy:
 * 1. Check E2E_UI_PORT env var
 * 2. Check for existing E2E containers and reuse their port
 * 3. Find first available port in range 3000-3019
 *
 * @returns Port number to use for E2E tests
 * @throws Error if no ports available
 */
export function selectE2EPort(): number {
  // Strategy 1: Check if E2E_UI_PORT is set
  if (process.env.E2E_UI_PORT) {
    const requestedPort = parseInt(process.env.E2E_UI_PORT, 10);

    try {
      if (checkPortAvailability(requestedPort)) {
        return requestedPort;
      }
      // If E2E_UI_PORT is set but occupied, skip container check and find next port
      // Start from requestedPort + 1 since we already checked requestedPort
      const port = findAvailablePort(requestedPort + 1, 3019);
      return port;
    } catch {
      // Port validation failed, continue to next strategy
    }
  }

  // Strategy 2: Check for existing E2E containers
  const containers = detectE2EContainers();
  if (containers.length > 0) {
    const existingPort = getPortFromContainers(containers);
    if (existingPort !== null) {
      return existingPort;
    }
  }

  // Strategy 3: Find first available port in range
  const port = findAvailablePort(3000, 3019);
  return port;
}

/**
 * Playwright global setup function.
 * Selects an available port and sets E2E_UI_PORT environment variable.
 */
export default async function globalSetup() {
  const port = selectE2EPort();
  process.env.E2E_UI_PORT = port.toString();
  console.log(`E2E tests will use port: ${port}`);
}
