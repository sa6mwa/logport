package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"text/tabwriter"
	"unicode"
)

type benchRow struct {
	fullName  string
	base      string
	variant   string
	ns        float64
	nsText    string
	bText     string
	aText     string
	bytes     float64
	bytesText string
}

type runData struct {
	rows []benchRow
}

func main() {
	bench := flag.String("bench", ".", "pattern passed to go test -bench")
	raw := flag.Bool("show-go-output", false, "print go test output for each run before the summary")
	runs := flag.Int("runs", 1, "number of go benchmark runs to execute")
	showCPU := flag.Bool("cpuinfo", true, "capture cpupower frequency-info before running benchmarks")
	benchtime := flag.String("benchtime", "1000x", "value passed to go test -benchtime (e.g. 1s, 500x)")
	nativeLogs := flag.Bool("native-logfiles", false, "write native benchmark output to <logger>.log files (requires native benchmarks)")
	flag.Parse()

	if *runs < 1 {
		*runs = 1
	}

	if *showCPU {
		printCPUInfo()
	}

	args, workdir := buildGoArgs(*bench, *benchtime, *nativeLogs)

	var datasets []runData
	for i := 1; i <= *runs; i++ {
		output, err := runGoBench(args, workdir)
		if err != nil {
			output.raw.WriteTo(os.Stderr)
			fmt.Fprintf(os.Stderr, "benchorder: failed to run %v: %v\n", strings.Join(args, " "), err)
			os.Exit(1)
		}
		rows := parseBenchOutput(output.raw.Bytes())
		if len(rows) == 0 {
			output.raw.WriteTo(os.Stderr)
			fmt.Fprintf(os.Stderr, "benchorder: no benchmarks detected in go test output (run %d)\n", i)
			os.Exit(1)
		}
		if *raw {
			fmt.Printf("=== go test output (run %d) ===\n", i)
			os.Stdout.Write(output.raw.Bytes())
		}
		datasets = append(datasets, runData{rows: rows})
		fmt.Printf("Run %d/%d results\n", i, *runs)
		printRuns(rows)
	}

	if *runs > 1 {
		fmt.Printf("Aggregate over %d runs\n", *runs)
		printAggregate(datasets)
	}
}

func buildGoArgs(bench, benchtime string, nativeLogs bool) ([]string, string) {
	args := []string{"go", "test", "-run=^$", "-bench", bench, "-benchmem"}
	if benchtime != "" {
		args = append(args, "-benchtime", benchtime)
	}
	if nativeLogs {
		args = append(args, "-native.bench.logfiles")
	}
	if flag.NArg() == 0 {
		if _, err := os.Stat("benchmark/go.mod"); err == nil {
			args = append(args, "./...")
			return args, "benchmark"
		}
		args = append(args, "./...")
	} else {
		args = append(args, flag.Args()...)
	}
	return args, ""
}

type benchOutput struct {
	raw bytes.Buffer
}

func runGoBench(args []string, dir string) (benchOutput, error) {
	cmd := exec.Command(args[0], args[1:]...)
	if dir != "" {
		cmd.Dir = dir
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return benchOutput{raw: out}, err
}

func printRuns(rows []benchRow) {
	grouped, order := groupRows(rows)
	for _, base := range order {
		fmt.Printf("%s (%d benchmarks, sorted by ns/op)\n", base, len(grouped[base]))
		printGroup(grouped[base])
		fmt.Println()
	}
}

func parseBenchOutput(data []byte) []benchRow {
	var rows []benchRow
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "Benchmark") {
			continue
		}
		if row, ok := parseBenchLine(line); ok {
			rows = append(rows, row)
		}
	}
	return rows
}

func parseBenchLine(line string) (benchRow, bool) {
	fields := strings.Fields(line)
	if len(fields) < 4 || !strings.HasPrefix(fields[0], "Benchmark") {
		return benchRow{}, false
	}
	name := trimProcSuffix(fields[0])
	base, variant := splitBase(name)
	row := benchRow{
		fullName:  name,
		base:      base,
		variant:   variant,
		nsText:    "-",
		bText:     "-",
		aText:     "-",
		bytesText: "-",
	}
	for i := 2; i < len(fields); i++ {
		unit := fields[i]
		if !strings.HasSuffix(unit, "/op") {
			continue
		}
		valueIdx := i - 1
		if valueIdx < 2 {
			continue
		}
		value := fields[valueIdx]
		switch unit {
		case "ns/op":
			row.ns = parseFloat(value)
			row.nsText = value
		case "B/op":
			row.bText = value
		case "allocs/op":
			row.aText = value
		case "bytes/op":
			row.bytes = parseFloat(value)
			row.bytesText = value
		}
	}
	if row.nsText == "-" {
		return benchRow{}, false
	}
	return row, true
}

func groupRows(rows []benchRow) (map[string][]benchRow, []string) {
	grouped := make(map[string][]benchRow)
	var order []string
	seen := make(map[string]bool)
	for _, row := range rows {
		if !seen[row.base] {
			order = append(order, row.base)
			seen[row.base] = true
		}
		grouped[row.base] = append(grouped[row.base], row)
	}
	for _, base := range order {
		sort.Slice(grouped[base], func(i, j int) bool {
			if grouped[base][i].ns == grouped[base][j].ns {
				return grouped[base][i].variant < grouped[base][j].variant
			}
			return grouped[base][i].ns < grouped[base][j].ns
		})
	}
	return grouped, order
}

func printGroup(rows []benchRow) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Rank\tVariant\tTime (ns/op)\tBytes/op\tB/op\tallocs/op")
	for idx, row := range rows {
		label := row.variant
		if label == "" {
			label = row.fullName
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\n", idx+1, label, row.nsText, row.bytesText, row.bText, row.aText)
	}
	tw.Flush()
}

func trimProcSuffix(name string) string {
	idx := strings.LastIndex(name, "-")
	if idx == -1 {
		return name
	}
	if idx+1 == len(name) {
		return name
	}
	if digitsOnly(name[idx+1:]) {
		return name[:idx]
	}
	return name
}

func digitsOnly(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return s != ""
}

func splitBase(name string) (string, string) {
	if i := strings.IndexRune(name, '/'); i != -1 {
		return name[:i], name[i+1:]
	}
	return name, ""
}

func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

func printCPUInfo() {
	fmt.Println("=== cpupower frequency-info ===")
	cmd := exec.Command("cpupower", "frequency-info")
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("cpupower frequency-info failed: %v\n", err)
	} else {
		fmt.Print(string(out))
	}
	fmt.Println("===============================")
}

type aggregateStat struct {
	base         string
	variant      string
	timeSamples  []float64
	bytesSamples []float64
	bText        string
	bytesText    string
	allocText    string
	best         float64
	worst        float64
}

func printAggregate(datasets []runData) {
	statMap := make(map[string]map[string]*aggregateStat)
	order := make([]string, 0)
	seen := make(map[string]bool)
	for _, data := range datasets {
		for _, row := range data.rows {
			if !seen[row.base] {
				order = append(order, row.base)
				seen[row.base] = true
			}
			if statMap[row.base] == nil {
				statMap[row.base] = make(map[string]*aggregateStat)
			}
			stat := statMap[row.base][row.variant]
			if stat == nil {
				stat = &aggregateStat{
					base:      row.base,
					variant:   row.variant,
					bText:     row.bText,
					bytesText: row.bytesText,
					allocText: row.aText,
					best:      row.ns,
					worst:     row.ns,
				}
				statMap[row.base][row.variant] = stat
			}
			stat.timeSamples = append(stat.timeSamples, row.ns)
			stat.bytesSamples = append(stat.bytesSamples, row.bytes)
			if row.ns < stat.best {
				stat.best = row.ns
				stat.bText = row.bText
				stat.bytesText = row.bytesText
				stat.allocText = row.aText
			}
			if row.ns > stat.worst {
				stat.worst = row.ns
			}
		}
	}

	for _, base := range order {
		stats := make([]*aggregateStat, 0, len(statMap[base]))
		for _, stat := range statMap[base] {
			stats = append(stats, stat)
		}
		sort.Slice(stats, func(i, j int) bool {
			mi := mean(stats[i].timeSamples)
			mj := mean(stats[j].timeSamples)
			if mi == mj {
				return stats[i].variant < stats[j].variant
			}
			return mi < mj
		})
		fmt.Printf("%s aggregate\n", base)
		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "Rank\tVariant\tMean ns/op\tBest ns/op\tWorst ns/op\tSamples\tBytes/op\tB/op\tallocs/op")
		for idx, stat := range stats {
			label := stat.variant
			if label == "" {
				label = stat.base
			}
			meanBytes := mean(stat.bytesSamples)
			bytesText := stat.bytesText
			if meanBytes > 0 {
				bytesText = fmt.Sprintf("%.2f", meanBytes)
			}
			fmt.Fprintf(tw, "%d\t%s\t%.2f\t%.2f\t%.2f\t%d\t%s\t%s\t%s\n",
				idx+1,
				label,
				mean(stat.timeSamples),
				stat.best,
				stat.worst,
				len(stat.timeSamples),
				bytesText,
				stat.bText,
				stat.allocText,
			)
		}
		tw.Flush()
		fmt.Println()
	}
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var total float64
	for _, v := range values {
		total += v
	}
	return total / float64(len(values))
}
