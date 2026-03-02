package throughput

import (
	"bufio"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ProfileAnalysis holds the parsed results from all captured pprof profiles.
type ProfileAnalysis struct {
	CPU        *CPUAnalysis       `json:"cpu,omitempty"`
	Heap       *HeapAnalysis      `json:"heap,omitempty"`
	Goroutines *GoroutineAnalysis `json:"goroutines,omitempty"`
	Summary    string             `json:"summary"`
	Warnings   []string           `json:"warnings,omitempty"`
}

// CPUAnalysis holds parsed CPU profile data.
type CPUAnalysis struct {
	TopFunctions []FunctionSample `json:"top_functions"`
	TotalSamples int              `json:"total_samples"`
	RawOutput    string           `json:"raw_output,omitempty"`
}

// HeapAnalysis holds parsed heap profile data.
type HeapAnalysis struct {
	TopAllocators []FunctionSample `json:"top_allocators"`
	HeapInUseMB   float64          `json:"heap_in_use_mb"`
	HeapAllocMB   float64          `json:"heap_alloc_mb"`
	RawOutput     string           `json:"raw_output,omitempty"`
}

// GoroutineAnalysis holds parsed goroutine profile data.
type GoroutineAnalysis struct {
	TotalCount int      `json:"total_count"`
	TopStacks  []string `json:"top_stacks,omitempty"`
	RawOutput  string   `json:"raw_output,omitempty"`
}

// FunctionSample represents a single function in a pprof text output.
type FunctionSample struct {
	Name      string  `json:"name"`
	FlatPct   float64 `json:"flat_pct"`
	CumPct    float64 `json:"cum_pct"`
	FlatValue string  `json:"flat_value"`
	CumValue  string  `json:"cum_value"`
}

// analyzeProfiles parses captured pprof files and produces a readable summary.
func analyzeProfiles(profileDir string) (*ProfileAnalysis, error) {
	analysis := &ProfileAnalysis{}

	// Try CPU profile
	cpuFile := findProfile(profileDir, "*-cpu.pprof")
	if cpuFile != "" {
		cpu, err := analyzeCPUProfile(cpuFile)
		if err == nil {
			analysis.CPU = cpu
		}
	}

	// Try heap profile (prefer final over baseline)
	heapFile := findProfile(profileDir, "*-final-heap.pprof")
	if heapFile == "" {
		heapFile = findProfile(profileDir, "*-heap.pprof")
	}
	if heapFile != "" {
		heap, err := analyzeHeapProfile(heapFile)
		if err == nil {
			analysis.Heap = heap
		}
	}

	// Try goroutine profile
	goroutineFile := findProfile(profileDir, "*-goroutine.pprof")
	if goroutineFile != "" {
		gr, err := analyzeGoroutineProfile(goroutineFile)
		if err == nil {
			analysis.Goroutines = gr
		}
	}

	// Generate warnings
	analysis.Warnings = generateWarnings(analysis)

	// Generate summary
	analysis.Summary = generateSummary(analysis)

	return analysis, nil
}

// findProfile finds the first matching profile file in the directory.
func findProfile(dir, pattern string) string {
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil || len(matches) == 0 {
		return ""
	}
	// Return the last match (most recent by lexicographic sort)
	sort.Strings(matches)
	return matches[len(matches)-1]
}

// analyzeCPUProfile runs go tool pprof -text on a CPU profile.
func analyzeCPUProfile(file string) (*CPUAnalysis, error) {
	output, err := runPprof(file, "-text", "-nodecount=15")
	if err != nil {
		return nil, err
	}

	functions := parsePprofTextOutput(output)
	return &CPUAnalysis{
		TopFunctions: functions,
		TotalSamples: len(functions),
		RawOutput:    output,
	}, nil
}

// analyzeHeapProfile runs go tool pprof -text on a heap profile.
func analyzeHeapProfile(file string) (*HeapAnalysis, error) {
	output, err := runPprof(file, "-text", "-nodecount=10", "-inuse_space")
	if err != nil {
		return nil, err
	}

	functions := parsePprofTextOutput(output)
	heap := &HeapAnalysis{
		TopAllocators: functions,
		RawOutput:     output,
	}

	// Try to parse total from header lines
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "flat") && strings.Contains(line, "cum") {
			// Header line, skip
			continue
		}
		// Look for "Showing nodes accounting for X of Y total"
		if strings.Contains(line, "total") && strings.Contains(line, "MB") {
			parts := strings.Fields(line)
			for i, p := range parts {
				if strings.HasSuffix(p, "MB") && i > 0 {
					val := strings.TrimSuffix(p, "MB")
					if v, err := strconv.ParseFloat(val, 64); err == nil {
						if heap.HeapInUseMB == 0 {
							heap.HeapInUseMB = v
						} else {
							heap.HeapAllocMB = v
						}
					}
				}
			}
		}
	}

	return heap, nil
}

// analyzeGoroutineProfile parses a goroutine profile.
func analyzeGoroutineProfile(file string) (*GoroutineAnalysis, error) {
	output, err := runPprof(file, "-text", "-nodecount=10")
	if err != nil {
		return nil, err
	}

	gr := &GoroutineAnalysis{
		RawOutput: output,
	}

	// Parse total goroutine count from pprof header
	// "Showing nodes accounting for X, Y% of Z total"
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "total") {
			re := regexp.MustCompile(`of\s+(\d+)\s+total`)
			if m := re.FindStringSubmatch(line); len(m) > 1 {
				if v, err := strconv.Atoi(m[1]); err == nil {
					gr.TotalCount = v
				}
			}
		}
	}

	// Collect top function stacks
	functions := parsePprofTextOutput(output)
	for _, f := range functions {
		if len(gr.TopStacks) < 5 {
			gr.TopStacks = append(gr.TopStacks, fmt.Sprintf("%s (%.1f%%)", f.Name, f.CumPct))
		}
	}

	return gr, nil
}

// runPprof executes `go tool pprof` with the given arguments.
func runPprof(file string, args ...string) (string, error) {
	cmdArgs := append([]string{"tool", "pprof"}, args...)
	cmdArgs = append(cmdArgs, file)
	cmd := exec.Command("go", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go tool pprof %s: %w\n%s", file, err, string(out))
	}
	return string(out), nil
}

// parsePprofTextOutput parses the tabular output from `go tool pprof -text`.
// Expected format:
//
//	flat  flat%   sum%   cum   cum%   name
//	10ms  5.00%  5.00%  20ms 10.00%  runtime.schedule
func parsePprofTextOutput(output string) []FunctionSample {
	var functions []FunctionSample
	scanner := bufio.NewScanner(strings.NewReader(output))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Skip header and comment lines
		if strings.HasPrefix(line, "flat") || strings.HasPrefix(line, "Showing") ||
			strings.HasPrefix(line, "Type:") || strings.HasPrefix(line, "Time:") ||
			strings.HasPrefix(line, "Duration:") || strings.HasPrefix(line, "Active") ||
			strings.HasPrefix(line, "Total") || strings.HasPrefix(line, "(pprof)") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}

		// Parse flat% and cum% (fields at index 1 and 4)
		flatPct := parsePercent(fields[1])
		cumPct := parsePercent(fields[4])

		// Only include if we got valid percentages
		if flatPct == 0 && cumPct == 0 {
			continue
		}

		functions = append(functions, FunctionSample{
			Name:      fields[5],
			FlatPct:   flatPct,
			CumPct:    cumPct,
			FlatValue: fields[0],
			CumValue:  fields[3],
		})
	}

	return functions
}

// parsePercent converts "23.50%" to 23.50.
func parsePercent(s string) float64 {
	s = strings.TrimSuffix(s, "%")
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// generateWarnings produces auto-flagged warnings from the analysis.
func generateWarnings(a *ProfileAnalysis) []string {
	var warnings []string

	if a.CPU != nil {
		for _, f := range a.CPU.TopFunctions {
			if f.CumPct > 30 {
				warnings = append(warnings, fmt.Sprintf(
					"%s uses %.1f%% cumulative CPU — investigate for contention",
					f.Name, f.CumPct))
			}
		}
		// Check for scheduling contention
		for _, f := range a.CPU.TopFunctions {
			if strings.Contains(f.Name, "runtime.findrunnable") && f.FlatPct > 10 {
				warnings = append(warnings, "runtime.findrunnable is high — goroutine scheduling contention")
			}
		}
	}

	if a.Heap != nil && a.Heap.HeapInUseMB > 50 {
		warnings = append(warnings, fmt.Sprintf(
			"Heap in use: %.1fMB — significant memory usage", a.Heap.HeapInUseMB))
	}

	if a.Goroutines != nil && a.Goroutines.TotalCount > 100 {
		warnings = append(warnings, fmt.Sprintf(
			"%d goroutines active — check for goroutine leaks", a.Goroutines.TotalCount))
	}

	return warnings
}

// generateSummary produces a human-readable paragraph summarizing the profile analysis.
func generateSummary(a *ProfileAnalysis) string {
	var b strings.Builder

	b.WriteString("Profile Analysis Summary:\n")

	if a.CPU != nil && len(a.CPU.TopFunctions) > 0 {
		b.WriteString("  CPU: Top consumers:")
		for i, f := range a.CPU.TopFunctions {
			if i >= 3 {
				break
			}
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(fmt.Sprintf(" %s (%.1f%%)", shortName(f.Name), f.CumPct))
		}
		b.WriteString("\n")
	} else {
		b.WriteString("  CPU: No profile captured\n")
	}

	if a.Heap != nil {
		if a.Heap.HeapInUseMB > 0 {
			b.WriteString(fmt.Sprintf("  Heap: %.1fMB in use", a.Heap.HeapInUseMB))
		} else if len(a.Heap.TopAllocators) > 0 {
			b.WriteString("  Heap: Top allocators:")
			for i, f := range a.Heap.TopAllocators {
				if i >= 3 {
					break
				}
				if i > 0 {
					b.WriteString(",")
				}
				b.WriteString(fmt.Sprintf(" %s (%s)", shortName(f.Name), f.FlatValue))
			}
		}
		b.WriteString("\n")
	} else {
		b.WriteString("  Heap: No profile captured\n")
	}

	if a.Goroutines != nil {
		b.WriteString(fmt.Sprintf("  Goroutines: %d active\n", a.Goroutines.TotalCount))
	} else {
		b.WriteString("  Goroutines: No profile captured\n")
	}

	if len(a.Warnings) > 0 {
		b.WriteString("  Warnings:\n")
		for _, w := range a.Warnings {
			b.WriteString(fmt.Sprintf("    - %s\n", w))
		}
	}

	return b.String()
}

// shortName returns the last component of a fully-qualified function name.
func shortName(name string) string {
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	return name
}

// printProfileAnalysis prints the profile analysis to stdout.
func printProfileAnalysis(a *ProfileAnalysis) {
	fmt.Println()
	fmt.Print(a.Summary)
}
