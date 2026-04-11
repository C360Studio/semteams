import { describe, it, expect } from "vitest";
import type {
  ComponentType,
  PortDefinition,
  PortConfig,
  NATSPortConfig,
  NATSRequestPortConfig,
  JetStreamPortConfig,
  KVWatchPortConfig,
  KVWritePortConfig,
  NetworkPortConfig,
  FilePortConfig,
  InterfaceContract,
  ConfigSchema,
  PropertySchema,
} from "./component";

describe("Component Type Definitions", () => {
  describe("ComponentType interface", () => {
    it("should match backend registry schema", () => {
      const component: ComponentType = {
        id: "udp-input",
        name: "UDP Input",
        type: "input",
        protocol: "udp",
        category: "inputs",
        description: "Receives UDP packets and publishes to NATS",
        version: "1.0.0",
        ports: [],
        schema: {
          type: "object",
          properties: {},
          required: [],
        },
      };

      expect(component.id).toBe("udp-input");
      expect(component.name).toBe("UDP Input");
      expect(component.type).toBe("input");
      expect(component.protocol).toBe("udp");
      expect(component.category).toBe("inputs");
      expect(component.version).toBe("1.0.0");
    });

    it("should support all component categories", () => {
      const categories: Array<ComponentType["category"]> = [
        "inputs",
        "processors",
        "outputs",
        "storage",
      ];

      categories.forEach((category) => {
        const component: ComponentType = {
          id: "test-component",
          name: "Test Component",
          type: "input",
          protocol: "test",
          category,
          description: "Test description",
          version: "1.0.0",
          ports: [],
          schema: {
            type: "object",
            properties: {},
            required: [],
          },
        };

        expect(component.category).toBe(category);
      });
    });

    it("should allow optional icon field", () => {
      const component: ComponentType = {
        id: "udp-input",
        name: "UDP Input",
        type: "input",
        protocol: "udp",
        category: "inputs",
        description: "Test",
        version: "1.0.0",
        ports: [],
        schema: {
          type: "object",
          properties: {},
          required: [],
        },
        icon: "/icons/udp.svg",
      };

      expect(component.icon).toBe("/icons/udp.svg");
    });
  });

  describe("PortDefinition interface", () => {
    it("should match backend port schema", () => {
      const port: PortDefinition = {
        id: "output",
        name: "Output Stream",
        direction: "output",
        required: true,
        description: "NATS output stream",
        config: {
          type: "nats",
          nats: {
            subject: "telemetry.raw",
          },
        },
      };

      expect(port.id).toBe("output");
      expect(port.direction).toBe("output");
      expect(port.required).toBe(true);
      expect(port.config.type).toBe("nats");
    });

    it("should support all port directions", () => {
      const directions: Array<PortDefinition["direction"]> = [
        "input",
        "output",
        "bidirectional",
      ];

      directions.forEach((direction) => {
        const port: PortDefinition = {
          id: "test-port",
          name: "Test Port",
          direction,
          required: false,
          description: "Test",
          config: {
            type: "nats",
            nats: { subject: "test.subject" },
          },
        };

        expect(port.direction).toBe(direction);
      });
    });
  });

  describe("PortConfig type discrimination", () => {
    it("should support NATS port config", () => {
      const natsConfig: NATSPortConfig = {
        subject: "telemetry.mavlink",
        queue: "workers",
      };

      const portConfig: PortConfig = {
        type: "nats",
        nats: natsConfig,
      };

      expect(portConfig.type).toBe("nats");
      expect(portConfig.nats?.subject).toBe("telemetry.mavlink");
      expect(portConfig.nats?.queue).toBe("workers");
    });

    it("should support NATS request port config", () => {
      const requestConfig: NATSRequestPortConfig = {
        subject: "entity.query",
        timeout: "5s",
        retries: 3,
      };

      const portConfig: PortConfig = {
        type: "nats-request",
        natsRequest: requestConfig,
      };

      expect(portConfig.type).toBe("nats-request");
      expect(portConfig.natsRequest?.subject).toBe("entity.query");
      expect(portConfig.natsRequest?.timeout).toBe("5s");
      expect(portConfig.natsRequest?.retries).toBe(3);
    });

    it("should support JetStream port config", () => {
      const jsConfig: JetStreamPortConfig = {
        streamName: "TELEMETRY",
        subjects: ["telemetry.*", "status.*"],
        consumerName: "processor-1",
      };

      const portConfig: PortConfig = {
        type: "jetstream",
        jetstream: jsConfig,
      };

      expect(portConfig.type).toBe("jetstream");
      expect(portConfig.jetstream?.streamName).toBe("TELEMETRY");
      expect(portConfig.jetstream?.subjects).toEqual([
        "telemetry.*",
        "status.*",
      ]);
    });

    it("should support KV watch port config", () => {
      const kvWatchConfig: KVWatchPortConfig = {
        bucket: "entity_states",
        keys: ["entity.*"],
      };

      const portConfig: PortConfig = {
        type: "kvwatch",
        kvwatch: kvWatchConfig,
      };

      expect(portConfig.type).toBe("kvwatch");
      expect(portConfig.kvwatch?.bucket).toBe("entity_states");
      expect(portConfig.kvwatch?.keys).toEqual(["entity.*"]);
    });

    it("should support KV write port config", () => {
      const kvWriteConfig: KVWritePortConfig = {
        bucket: "entity_states",
      };

      const portConfig: PortConfig = {
        type: "kvwrite",
        kvwrite: kvWriteConfig,
      };

      expect(portConfig.type).toBe("kvwrite");
      expect(portConfig.kvwrite?.bucket).toBe("entity_states");
    });

    it("should support network port config", () => {
      const networkConfig: NetworkPortConfig = {
        protocol: "udp",
        host: "0.0.0.0",
        port: 14550,
      };

      const portConfig: PortConfig = {
        type: "network",
        network: networkConfig,
      };

      expect(portConfig.type).toBe("network");
      expect(portConfig.network?.protocol).toBe("udp");
      expect(portConfig.network?.port).toBe(14550);
    });

    it("should support file port config", () => {
      const fileConfig: FilePortConfig = {
        path: "/data/logs",
        pattern: "*.log",
      };

      const portConfig: PortConfig = {
        type: "file",
        file: fileConfig,
      };

      expect(portConfig.type).toBe("file");
      expect(portConfig.file?.path).toBe("/data/logs");
      expect(portConfig.file?.pattern).toBe("*.log");
    });
  });

  describe("InterfaceContract interface", () => {
    it("should define interface compatibility", () => {
      const contract: InterfaceContract = {
        type: "message.Storable",
        version: "v1",
        compatible: ["message.StorableV2", "message.Entity"],
      };

      expect(contract.type).toBe("message.Storable");
      expect(contract.version).toBe("v1");
      expect(contract.compatible).toEqual([
        "message.StorableV2",
        "message.Entity",
      ]);
    });

    it("should allow optional version and compatible fields", () => {
      const contract: InterfaceContract = {
        type: "message.Generic",
      };

      expect(contract.type).toBe("message.Generic");
      expect(contract.version).toBeUndefined();
      expect(contract.compatible).toBeUndefined();
    });
  });

  describe("ConfigSchema interface", () => {
    it("should define component configuration schema", () => {
      const schema: ConfigSchema = {
        type: "object",
        properties: {
          port: {
            type: "number",
            description: "UDP port to listen on",
            minimum: 1,
            maximum: 65535,
            default: 14550,
          },
          host: {
            type: "string",
            description: "Host address to bind",
            default: "0.0.0.0",
          },
        },
        required: ["port"],
      };

      expect(schema.type).toBe("object");
      expect(schema.properties.port.type).toBe("number");
      expect(schema.properties.port.minimum).toBe(1);
      expect(schema.properties.port.maximum).toBe(65535);
      expect(schema.required).toEqual(["port"]);
    });
  });

  describe("PropertySchema interface", () => {
    it("should support all property types", () => {
      const types: Array<PropertySchema["type"]> = [
        "string",
        "number",
        "boolean",
        "array",
        "object",
      ];

      types.forEach((type) => {
        const prop: PropertySchema = {
          type,
          description: `Test ${type} property`,
        };

        expect(prop.type).toBe(type);
      });
    });

    it("should support enum constraints", () => {
      const prop: PropertySchema = {
        type: "string",
        description: "Log level",
        enum: ["debug", "info", "warn", "error"],
        default: "info",
      };

      expect(prop.enum).toEqual(["debug", "info", "warn", "error"]);
      expect(prop.default).toBe("info");
    });

    it("should support numeric constraints", () => {
      const prop: PropertySchema = {
        type: "number",
        description: "Port number",
        minimum: 1,
        maximum: 65535,
        default: 8080,
      };

      expect(prop.minimum).toBe(1);
      expect(prop.maximum).toBe(65535);
      expect(prop.default).toBe(8080);
    });
  });
});
