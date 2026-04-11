import { describe, it, expect } from "vitest";
import type { SaveState, RuntimeStateInfo, UserPreferences } from "./ui-state";
import { DEFAULT_PREFERENCES } from "./ui-state";

describe("UI State Types", () => {
  describe("SaveState", () => {
    it("should have correct structure for clean state", () => {
      const state: SaveState = {
        status: "clean",
        lastSaved: new Date(),
        error: null,
      };

      expect(state.status).toBe("clean");
      expect(state.lastSaved).toBeInstanceOf(Date);
      expect(state.error).toBeNull();
    });

    it("should have correct structure for dirty state", () => {
      const state: SaveState = {
        status: "dirty",
        lastSaved: new Date(),
        error: null,
      };

      expect(state.status).toBe("dirty");
      expect(state.lastSaved).toBeInstanceOf(Date);
    });

    it("should have correct structure for saving state", () => {
      const state: SaveState = {
        status: "saving",
        lastSaved: null,
        error: null,
      };

      expect(state.status).toBe("saving");
    });

    it("should have correct structure for error state", () => {
      const state: SaveState = {
        status: "error",
        lastSaved: null,
        error: "Network error",
      };

      expect(state.status).toBe("error");
      expect(state.error).toBe("Network error");
    });
  });

  describe("RuntimeStateInfo", () => {
    it("should have correct structure with all runtime states", () => {
      const states: RuntimeStateInfo[] = [
        { state: "not_deployed", message: null, lastTransition: null },
        {
          state: "deployed_stopped",
          message: null,
          lastTransition: new Date(),
        },
        {
          state: "running",
          message: "All components running",
          lastTransition: new Date(),
        },
        {
          state: "error",
          message: "Component crashed",
          lastTransition: new Date(),
        },
      ];

      states.forEach((state) => {
        expect(state.state).toMatch(
          /not_deployed|deployed_stopped|running|error/,
        );
        if (state.lastTransition) {
          expect(state.lastTransition).toBeInstanceOf(Date);
        }
      });
    });
  });

  describe("UserPreferences", () => {
    it("should have default preferences object", () => {
      expect(DEFAULT_PREFERENCES).toBeDefined();
      expect(typeof DEFAULT_PREFERENCES).toBe("object");
    });

    it("should accept empty preferences (ready for theme)", () => {
      const prefs: UserPreferences = {};
      expect(prefs).toBeDefined();
    });
  });
});
