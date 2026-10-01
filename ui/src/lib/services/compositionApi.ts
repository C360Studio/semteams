// The retained component endpoints project the admitted boot composition.
export interface CompositionNode {
  instance: string;
  factory: string;
  type: string;
  inputs: unknown[];
  outputs: unknown[];
}
export interface CompositionGraph {
  nodes: CompositionNode[];
  edges: Array<{ from: string; to: string; from_port: string; to_port: string }>;
}
export interface CompositionFinding {
  component: string;
  message: string;
  severity: string;
  type: string;
  suggestions: string[];
}
export interface CompositionValidation {
  status: string;
  errors: CompositionFinding[];
  warnings: CompositionFinding[];
  graph: CompositionGraph;
}
export async function getComposition(request: typeof fetch = fetch): Promise<{
  graph: CompositionGraph;
  validation: CompositionValidation;
}> {
  const [graph, validation] = await Promise.all([
    request("/components/flowgraph"),
    request("/components/validate"),
  ]);
  for (const response of [graph, validation]) {
    if (!response.ok) throw new Error(`Composition unavailable (${response.status}): ${response.statusText}`);
  }
  return { graph: await graph.json(), validation: await validation.json() };
}
