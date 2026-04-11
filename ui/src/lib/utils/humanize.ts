/**
 * Humanizes a snake_case field name into Title Case.
 * Preserves common acronyms in uppercase.
 * Also handles mixed case input (e.g., MixedCase_Field -> Mixed Case Field)
 */
export function humanizeFieldName(name: string): string {
  // Handle empty string
  if (name === "") return "";

  // List of acronyms to preserve in uppercase
  const acronyms = new Set([
    "IP",
    "UDP",
    "TCP",
    "HTTP",
    "HTTPS",
    "URL",
    "API",
    "TTL",
    "ID",
    "NATS",
    "WS",
  ]);

  // First, split by underscores, filter out empty parts (handles leading/trailing/multiple underscores)
  const underscoreParts = name.split("_").filter((part) => part.length > 0);

  // Then, split each part by capital letters (camelCase/PascalCase)
  const words: string[] = [];
  for (const part of underscoreParts) {
    // Split on transitions to uppercase: "MixedCase" -> ["Mixed", "Case"]
    // Uses regex to insert a space before each capital letter, then split
    const camelWords = part
      .replace(/([a-z])([A-Z])/g, "$1 $2") // lowercase followed by uppercase
      .replace(/([A-Z]+)([A-Z][a-z])/g, "$1 $2") // handle consecutive caps like "XMLParser"
      .split(" ")
      .filter((w) => w.length > 0);
    words.push(...camelWords);
  }

  // Process each word
  const processedWords = words.map((word) => {
    // Convert to lowercase for processing
    const lowerWord = word.toLowerCase();

    // Check if it's an acronym
    const upperWord = word.toUpperCase();
    if (acronyms.has(upperWord)) {
      return upperWord;
    }

    // Handle special case: numbers with letters (e.g., "ipv4" -> "IPv4")
    // Check if word has digits
    if (/\d/.test(lowerWord)) {
      // Check if it starts with a known acronym pattern
      for (const acronym of acronyms) {
        const lowerAcronym = acronym.toLowerCase();
        if (lowerWord.startsWith(lowerAcronym)) {
          const suffix = lowerWord.slice(lowerAcronym.length);
          // Keep acronym uppercase, append suffix as-is (lowercase)
          // e.g., "ipv4" -> "IP" + "v4" = "IPv4"
          return acronym + suffix;
        }
      }
    }

    // Standard title case: capitalize first letter
    return lowerWord.charAt(0).toUpperCase() + lowerWord.slice(1);
  });

  return processedWords.join(" ");
}
