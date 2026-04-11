import { describe, it, expect } from "vitest";
import {
  groupPorts,
  computePortVisualStyle,
  extractTooltipContent,
  checkPortCompatibility,
} from "./port-utils";
import type { ValidatedPort } from "$lib/types/port";

describe("port-utils", () => {
  describe("groupPorts", () => {
    const createPort = (
      name: string,
      direction: "input" | "output",
      type: string = "nats_stream",
    ): ValidatedPort => ({
      name,
      direction,
      type,
      required: false,
      connection_id: `test.${name}`,
      pattern: "stream",
      description: `${name} description`,
    });

    it("should group ports by direction", () => {
      const ports: ValidatedPort[] = [
        createPort("input1", "input"),
        createPort("input2", "input"),
        createPort("input3", "input"),
        createPort("output1", "output"),
        createPort("output2", "output"),
      ];

      const groups = groupPorts(ports);

      expect(groups).toHaveLength(2);

      const inputGroup = groups.find((g) => g.id === "inputs");
      const outputGroup = groups.find((g) => g.id === "outputs");

      expect(inputGroup).toBeTruthy();
      expect(outputGroup).toBeTruthy();
      expect(inputGroup?.ports).toHaveLength(3);
      expect(outputGroup?.ports).toHaveLength(2);
    });

    it("should position input group on left", () => {
      const ports: ValidatedPort[] = [
        createPort("input1", "input"),
        createPort("output1", "output"),
      ];

      const groups = groupPorts(ports);

      const inputGroup = groups.find((g) => g.id === "inputs");
      expect(inputGroup?.position).toBe("left");
    });

    it("should position output group on right", () => {
      const ports: ValidatedPort[] = [
        createPort("input1", "input"),
        createPort("output1", "output"),
      ];

      const groups = groupPorts(ports);

      const outputGroup = groups.find((g) => g.id === "outputs");
      expect(outputGroup?.position).toBe("right");
    });

    it("should remove empty groups", () => {
      const ports: ValidatedPort[] = [
        createPort("input1", "input"),
        createPort("input2", "input"),
        // No outputs
      ];

      const groups = groupPorts(ports);

      expect(groups).toHaveLength(1);
      expect(groups[0].id).toBe("inputs");
    });

    it("should default groups to expanded (collapsed=false)", () => {
      const ports: ValidatedPort[] = [
        createPort("input1", "input"),
        createPort("output1", "output"),
      ];

      const groups = groupPorts(ports);

      groups.forEach((group) => {
        expect(group.collapsed).toBe(false);
      });
    });

    it("should include port count in label", () => {
      const ports: ValidatedPort[] = [
        createPort("input1", "input"),
        createPort("input2", "input"),
        createPort("input3", "input"),
      ];

      const groups = groupPorts(ports);

      const inputGroup = groups.find((g) => g.id === "inputs");
      expect(inputGroup?.label).toContain("3");
      expect(inputGroup?.label).toContain("Input");
    });

    it("should handle empty port array", () => {
      const ports: ValidatedPort[] = [];

      const groups = groupPorts(ports);

      expect(groups).toHaveLength(0);
    });
  });

  // ============================================================================
  // Phase 3: Port Visual Styling Tests (Spec 014)
  // ============================================================================

  describe("computePortVisualStyle", () => {
    it("should return blue color for NATS stream port", () => {
      const port: ValidatedPort = {
        name: "nats_output",
        direction: "output",
        required: true,
        pattern: "stream", // Backend pattern type
        type: "message.Storable", // Interface contract type
        connection_id: "input.udp.mavlink", // NATS subject
        description: "NATS stream output",
      };

      const style = computePortVisualStyle(port);

      expect(style.color).toBe("var(--port-pattern-stream)"); // CSS variable
      expect(style.borderPattern).toBe("solid"); // Required
      expect(style.iconName).toBe("arrow-path-rounded-square");
      expect(style.ariaLabel).toContain("output");
      expect(style.ariaLabel).toContain("required");
      expect(style.cssClasses).toContain("port-handle");
    });

    it("should return dashed border for optional port", () => {
      const port: ValidatedPort = {
        name: "optional_output",
        direction: "output",
        required: false,
        pattern: "stream", // Backend pattern type
        type: "message.Storable", // Interface contract type
        connection_id: "telemetry.*", // NATS subject
        description: "Optional output",
      };

      const style = computePortVisualStyle(port);
      expect(style.borderPattern).toBe("dashed");
    });

    it("should return purple color for NATS request port", () => {
      const port: ValidatedPort = {
        name: "request_port",
        direction: "input",
        required: true,
        pattern: "request", // Backend pattern type
        type: "message.Request", // Interface contract type
        connection_id: "request.test", // NATS subject
        description: "Request port",
      };

      const style = computePortVisualStyle(port);
      expect(style.color).toBe("var(--port-pattern-request)"); // CSS variable
      expect(style.iconName).toBe("arrow-path");
    });

    it("should return green color for KV watch port", () => {
      const port: ValidatedPort = {
        name: "kv_watch",
        direction: "input",
        required: true,
        pattern: "watch", // Backend pattern type
        type: "kv.Entry", // Interface contract type
        connection_id: "BUCKET",
        description: "KV watch port",
      };

      const style = computePortVisualStyle(port);
      expect(style.color).toBe("var(--port-pattern-watch)"); // CSS variable
      expect(style.iconName).toBe("eye");
    });

    it("should return orange color for network port", () => {
      const port: ValidatedPort = {
        name: "udp_input",
        direction: "input",
        required: true,
        pattern: "api", // Backend pattern type for network/API
        type: "bytes", // Interface contract type
        connection_id: "0.0.0.0:8080", // Network address
        description: "UDP input",
      };

      const style = computePortVisualStyle(port);
      expect(style.color).toBe("var(--port-pattern-api)"); // CSS variable
      expect(style.iconName).toBe("server");
    });

    it("should return gray color for file port", () => {
      const port: ValidatedPort = {
        name: "file_input",
        direction: "input",
        required: true,
        pattern: "unknown", // Unknown pattern
        type: "bytes", // Interface contract type
        connection_id: "file:///data/*.csv", // File:// prefix indicates file
        description: "File input",
      };

      const style = computePortVisualStyle(port);
      expect(style.color).toBe("var(--port-pattern-file)"); // CSS variable
      expect(style.iconName).toBe("document-text");
    });

    it("should handle unknown port type with default color", () => {
      const port: ValidatedPort = {
        name: "unknown_port",
        direction: "input",
        required: true,
        pattern: "unknown", // Unknown pattern
        type: "unknown_type", // Interface contract type
        connection_id: "unknown", // No file:// prefix
        description: "Unknown port",
      };

      const style = computePortVisualStyle(port);
      expect(style.color).toBe("var(--port-pattern-api)"); // Default to network (CSS variable)
    });
  });

  describe("extractTooltipContent", () => {
    it("should extract all metadata from ValidatedPort", () => {
      const port: ValidatedPort = {
        name: "nats_output",
        direction: "output",
        required: true,
        pattern: "stream", // Backend pattern type
        type: "message.Storable", // Interface contract type
        connection_id: "input.udp.mavlink", // NATS subject
        description: "Publishes UAV telemetry",
      };

      const tooltip = extractTooltipContent(port);

      expect(tooltip.name).toBe("nats_output");
      expect(tooltip.type).toBe("NATS Stream"); // Display name
      expect(tooltip.pattern).toBe("input.udp.mavlink"); // Connection ID
      expect(tooltip.requirement).toBe("required");
      expect(tooltip.description).toBe("Publishes UAV telemetry");
    });

    it("should handle optional port", () => {
      const port: ValidatedPort = {
        name: "optional_port",
        direction: "input",
        required: false,
        pattern: "test.*",
        type: "nats_stream",
        connection_id: "test.*",
        description: "Optional port",
      };

      const tooltip = extractTooltipContent(port);
      expect(tooltip.requirement).toBe("optional");
    });

    it("should handle missing description", () => {
      const port: ValidatedPort = {
        name: "no_desc",
        direction: "input",
        required: true,
        pattern: "test",
        type: "nats_stream",
        connection_id: "test",
        description: "",
      };

      const tooltip = extractTooltipContent(port);
      expect(tooltip.description).toBe("");
    });
  });

  describe("checkPortCompatibility", () => {
    it("should return compatible for output→input with matching type", () => {
      const source: ValidatedPort = {
        name: "out",
        direction: "output",
        required: true,
        pattern: "test.*",
        type: "nats_stream",
        connection_id: "test.data",
        description: "Output",
      };
      const target: ValidatedPort = {
        name: "in",
        direction: "input",
        required: true,
        pattern: "test.*",
        type: "nats_stream",
        connection_id: "test.*",
        description: "Input",
      };

      const feedback = checkPortCompatibility(source, target);

      expect(feedback.compatibility).toBe("compatible");
      expect(feedback.indicator).toBe("green-highlight");
      expect(feedback.feedbackClasses).toContain("feedback-compatible");
    });

    it("should return incompatible for output→output", () => {
      const source: ValidatedPort = {
        name: "out1",
        direction: "output",
        required: true,
        pattern: "test.*",
        type: "nats_stream",
        connection_id: "test.data",
        description: "Output 1",
      };
      const target: ValidatedPort = {
        name: "out2",
        direction: "output",
        required: true,
        pattern: "test.*",
        type: "nats_stream",
        connection_id: "test.other",
        description: "Output 2",
      };

      const feedback = checkPortCompatibility(source, target);

      expect(feedback.compatibility).toBe("incompatible");
      expect(feedback.indicator).toBe("red-indicator");
      expect(feedback.incompatibilityReason).toContain("direction");
      expect(feedback.feedbackClasses).toContain("feedback-incompatible");
    });

    it("should return incompatible for type mismatch", () => {
      const source: ValidatedPort = {
        name: "nats_out",
        direction: "output",
        required: true,
        pattern: "test.*",
        type: "nats_stream",
        connection_id: "test.data",
        description: "NATS output",
      };
      const target: ValidatedPort = {
        name: "file_in",
        direction: "input",
        required: true,
        pattern: "/data/*.csv",
        type: "file",
        connection_id: "/data/*.csv",
        description: "File input",
      };

      const feedback = checkPortCompatibility(source, target);

      expect(feedback.compatibility).toBe("incompatible");
      expect(feedback.indicator).toBe("red-indicator");
      expect(feedback.incompatibilityReason).toContain("type");
    });

    it("should return incompatible for input→input", () => {
      const source: ValidatedPort = {
        name: "in1",
        direction: "input",
        required: true,
        pattern: "test.*",
        type: "nats_stream",
        connection_id: "test.*",
        description: "Input 1",
      };
      const target: ValidatedPort = {
        name: "in2",
        direction: "input",
        required: true,
        pattern: "test.*",
        type: "nats_stream",
        connection_id: "test.*",
        description: "Input 2",
      };

      const feedback = checkPortCompatibility(source, target);

      expect(feedback.compatibility).toBe("incompatible");
      expect(feedback.indicator).toBe("red-indicator");
    });
  });
});
