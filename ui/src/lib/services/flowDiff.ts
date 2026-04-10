import type { FlowNode, FlowConnection } from "$lib/types/flow";
import type { FlowDiff } from "$lib/types/chat";

export function computeFlowDiff(
  oldNodes: FlowNode[],
  oldConnections: FlowConnection[],
  newNodes: FlowNode[],
  newConnections: FlowConnection[],
): FlowDiff {
  const oldNodeMap = new Map(oldNodes.map((n) => [n.id, n]));
  const newNodeMap = new Map(newNodes.map((n) => [n.id, n]));

  const nodesAdded: string[] = [];
  const nodesRemoved: string[] = [];
  const nodesModified: string[] = [];

  // Detect added nodes (in new but not in old)
  for (const [id, newNode] of newNodeMap) {
    if (!oldNodeMap.has(id)) {
      nodesAdded.push(newNode.name);
    }
  }

  // Detect removed nodes (in old but not in new)
  for (const [id, oldNode] of oldNodeMap) {
    if (!newNodeMap.has(id)) {
      nodesRemoved.push(oldNode.name);
    }
  }

  // Detect modified nodes (in both, but config or name changed)
  for (const [id, newNode] of newNodeMap) {
    const oldNode = oldNodeMap.get(id);
    if (!oldNode) continue; // already counted as added

    const nameChanged = oldNode.name !== newNode.name;
    const configChanged =
      JSON.stringify(oldNode.config) !== JSON.stringify(newNode.config);

    if (nameChanged || configChanged) {
      nodesModified.push(newNode.name);
    }
  }

  // Connection diffing by id
  const oldConnIds = new Set(oldConnections.map((c) => c.id));
  const newConnIds = new Set(newConnections.map((c) => c.id));

  let connectionsAdded = 0;
  let connectionsRemoved = 0;

  for (const id of newConnIds) {
    if (!oldConnIds.has(id)) connectionsAdded++;
  }
  for (const id of oldConnIds) {
    if (!newConnIds.has(id)) connectionsRemoved++;
  }

  return {
    nodesAdded,
    nodesRemoved,
    nodesModified,
    connectionsAdded,
    connectionsRemoved,
  };
}
