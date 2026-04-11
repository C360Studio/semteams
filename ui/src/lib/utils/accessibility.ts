/**
 * Accessibility utilities for WCAG AA compliance
 *
 * Provides ARIA label generation, color contrast verification,
 * and screen reader announcements for port visualization.
 *
 * @see specs/014-flow-ux-port/research.md (section 8)
 */

import type { ValidatedPort, ValidationResult } from "../types/port";

/**
 * Generate ARIA label for port handle
 *
 * Creates descriptive label for screen readers including port type,
 * direction, name, and requirement status.
 *
 * Example: "nats stream Output: nats_output (required)"
 *
 * @param port - ValidatedPort from backend
 * @returns Descriptive ARIA label string
 */
export function generatePortAriaLabel(port: ValidatedPort): string {
  // Format type label (replace underscores with spaces, capitalize)
  const typeLabel = port.type ? port.type.replace(/_/g, " ") : "Unknown";

  // Capitalize direction
  const directionLabel = port.direction === "input" ? "Input" : "Output";

  // Requirement status
  const requirementLabel = port.required ? "required" : "optional";

  // Format: "Type Direction: name (requirement)"
  return `${typeLabel} ${directionLabel}: ${port.name} (${requirementLabel})`;
}

/**
 * Verify color contrast ratio meets WCAG AA standard
 *
 * Calculates relative luminance and contrast ratio according to WCAG 2.1
 * specification. WCAG AA requires minimum 4.5:1 for normal text.
 *
 * @param foreground - Foreground color (hex format, e.g., "#3B82F6")
 * @param background - Background color (hex format, e.g., "#FFFFFF")
 * @returns True if contrast ratio >= 4.5:1 (WCAG AA compliant)
 */
export function verifyColorContrast(
  foreground: string,
  background: string,
): boolean {
  // Convert hex to RGB
  const hexToRgb = (hex: string): [number, number, number] => {
    const cleaned = hex.replace("#", "");
    const r = parseInt(cleaned.substring(0, 2), 16);
    const g = parseInt(cleaned.substring(2, 4), 16);
    const b = parseInt(cleaned.substring(4, 6), 16);
    return [r, g, b];
  };

  // Calculate relative luminance (WCAG formula)
  const getLuminance = (rgb: [number, number, number]): number => {
    const [r, g, b] = rgb.map((val) => {
      const sRGB = val / 255;
      return sRGB <= 0.03928
        ? sRGB / 12.92
        : Math.pow((sRGB + 0.055) / 1.055, 2.4);
    });
    return 0.2126 * r + 0.7152 * g + 0.0722 * b;
  };

  // Calculate contrast ratio
  const fgLuminance = getLuminance(hexToRgb(foreground));
  const bgLuminance = getLuminance(hexToRgb(background));

  const lighter = Math.max(fgLuminance, bgLuminance);
  const darker = Math.min(fgLuminance, bgLuminance);
  const contrastRatio = (lighter + 0.05) / (darker + 0.05);

  // WCAG AA requires 4.5:1 for normal text
  return contrastRatio >= 4.5;
}

/**
 * Generate screen reader announcement for validation results
 *
 * Creates concise announcement of validation status for ARIA live regions.
 * Prioritizes errors over warnings, announces counts for context.
 *
 * @param validationResult - ValidationResult from flow validation
 * @returns Screen reader announcement string
 */
export function generateValidationAnnouncement(
  validationResult: ValidationResult,
): string {
  const errorCount = validationResult.errors?.length || 0;
  const warningCount = validationResult.warnings?.length || 0;

  if (errorCount > 0) {
    const errorText = errorCount === 1 ? "error" : "errors";
    return `Validation failed with ${errorCount} ${errorText}`;
  }

  if (warningCount > 0) {
    const warningText = warningCount === 1 ? "warning" : "warnings";
    return `Validation passed with ${warningCount} ${warningText}`;
  }

  return "Validation passed";
}
