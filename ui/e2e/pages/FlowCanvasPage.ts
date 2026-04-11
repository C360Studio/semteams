import { Page, Locator, expect } from "@playwright/test";

/**
 * Page Object Model for the Flow Canvas editor page
 * Encapsulates interactions with the visual flow editor
 */
export class FlowCanvasPage {
  constructor(private page: Page) {}

  // Locators
  get canvas(): Locator {
    return this.page.locator("#flow-canvas");
  }

  get saveButton(): Locator {
    return this.page.locator('button:has-text("Save")');
  }

  get saveStatus(): Locator {
    return this.page.locator("#save-status");
  }

  get backButton(): Locator {
    return this.page.locator('a:has-text("← Flows")');
  }

  get nodes(): Locator {
    return this.page.locator("[data-node-id]");
  }

  get edges(): Locator {
    return this.page.locator("g.flow-edge");
  }

  getAutoEdges(): Locator {
    return this.page.locator('g.flow-edge[data-source="auto"]');
  }

  getManualEdges(): Locator {
    return this.page.locator('g.flow-edge[data-source="manual"]');
  }

  getEdgeById(edgeId: string): Locator {
    return this.page.locator(`g.flow-edge[data-connection-id="${edgeId}"]`);
  }

  // Port locators
  getInputPorts(nodeId: string): Locator {
    return this.getNodeById(nodeId).locator(".port-input");
  }

  getOutputPorts(nodeId: string): Locator {
    return this.getNodeById(nodeId).locator(".port-output");
  }

  getPortByName(nodeId: string, portName: string): Locator {
    return this.getNodeById(nodeId).locator(`[data-port-name="${portName}"]`);
  }

  // Actions
  async goto(flowId: string): Promise<void> {
    await this.page.goto(`/flows/${flowId}`);
  }

  getNodeByType(type: string): Locator {
    return this.page.locator(`[data-node-id][data-node-type="${type}"]`);
  }

  getNodeById(nodeId: string): Locator {
    return this.page.locator(`[data-node-id="${nodeId}"]`);
  }

  async clickNode(nodeId: string): Promise<void> {
    const node = this.getNodeById(nodeId);
    await node.click();
  }

  /**
   * Click the edit button for a component in the sidebar
   * This opens the EditComponentModal
   */
  async clickEditButton(nodeName: string): Promise<void> {
    const editButton = this.page.locator(
      `button[aria-label="Edit ${nodeName}"]`,
    );
    await editButton.click();
  }

  /**
   * Get the first node's name from the sidebar
   */
  async getFirstNodeName(): Promise<string> {
    const firstCard = this.page.locator(".component-card").first();
    const nameElement = firstCard.locator("h4");
    return (await nameElement.textContent()) || "";
  }

  async clickSaveButton(): Promise<void> {
    await this.saveButton.click();

    // Wait for the save request to complete by waiting for network idle
    await this.page.waitForLoadState("networkidle");
  }

  async clickBackButton(): Promise<void> {
    await this.backButton.click();
  }

  // Assertions
  async expectCanvasLoaded(): Promise<void> {
    await expect(this.canvas).toBeVisible();
  }

  async expectNodeAtPosition(
    nodeId: string,
    x: number,
    y: number,
  ): Promise<void> {
    const node = this.getNodeById(nodeId);
    const box = await node.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.x).toBeCloseTo(x, 0);
    expect(box!.y).toBeCloseTo(y, 0);
  }

  async expectSaveStatus(
    status: "clean" | "dirty" | "saving" | "draft",
  ): Promise<void> {
    await expect(this.saveStatus).toHaveAttribute("data-status", status);
  }

  async expectNodeCount(count: number): Promise<void> {
    await expect(this.nodes).toHaveCount(count);
  }

  async expectNodeVisible(nodeId: string): Promise<void> {
    const node = this.getNodeById(nodeId);
    await expect(node).toBeVisible();
  }
}
