/**
 * ConfigValue - Recursive type for component configuration values.
 *
 * Replaces `any` across config-handling components. Config values can be
 * primitives, arrays of config values, or objects with config value entries.
 */
export type ConfigValue =
  | string
  | number
  | boolean
  | null
  | ConfigValue[]
  | { [key: string]: ConfigValue };
