package vmm

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"litevm/internal/i18n"
)

const (
	s3BaseURL        = "https://s3.amazonaws.com/spec.ccfc.min"
	quickstartPrefix = "img/quickstart_guide/"
	ciPrefix         = "firecracker-ci/"
)

type s3ListResult struct {
	XMLName        xml.Name         `xml:"ListBucketResult"`
	Contents       []s3Content      `xml:"Contents"`
	CommonPrefixes []s3CommonPrefix `xml:"CommonPrefixes"`
}

type s3Content struct {
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	Size         int64  `xml:"Size"`
}

type s3CommonPrefix struct {
	Prefix string `xml:"Prefix"`
}

type KernelInfo struct {
	Name    string
	Key     string
	Size    int64
	ModTime string
	Source  string
	Version string
	Arch    string
}

var supportedArchs = []string{"x86_64", "aarch64"}

type progressWriter struct {
	total   int64
	current int64
	lastPct int
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.current += int64(n)

	if pw.total > 0 {
		pct := int(pw.current * 100 / pw.total)
		if pct != pw.lastPct {
			pw.lastPct = pct
			barLen := 30
			filled := pct * barLen / 100
			bar := strings.Repeat("=", filled) + strings.Repeat(" ", barLen-filled)
			fmt.Printf("\r  [%s] %3d%% %s/%s", bar, pct, formatSize(pw.current), formatSize(pw.total))
		}
	} else {
		fmt.Printf("\r  Downloaded %s", formatSize(pw.current))
	}
	return n, nil
}

func listQuickstartKernels(arch string) ([]KernelInfo, error) {
	prefix := fmt.Sprintf("%s%s/kernels/", quickstartPrefix, arch)
	url := fmt.Sprintf("%s/?list-type=2&prefix=%s&delimiter=/", s3BaseURL, prefix)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result s3ListResult
	if err := xml.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	var kernels []KernelInfo
	for _, c := range result.Contents {
		if !strings.HasSuffix(c.Key, ".bin") {
			continue
		}
		name := filepath.Base(c.Key)
		kernels = append(kernels, KernelInfo{
			Name:    name,
			Key:     c.Key,
			Size:    c.Size,
			ModTime: c.LastModified[:10],
			Source:  "quickstart",
			Version: strings.TrimSuffix(strings.TrimPrefix(name, "vmlinux"), ".bin"),
			Arch:    arch,
		})
	}
	return kernels, nil
}

func listLatestCIBuild() (string, error) {
	url := fmt.Sprintf("%s/?list-type=2&prefix=%s&delimiter=/", s3BaseURL, ciPrefix)
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result s3ListResult
	if err := xml.Unmarshal(body, &result); err != nil {
		return "", err
	}

	var latest string
	for _, p := range result.CommonPrefixes {
		parts := strings.TrimPrefix(p.Prefix, ciPrefix)
		parts = strings.TrimSuffix(parts, "/")
		if len(parts) >= 8 {
			datePart := parts[:8]
			if _, err := strconv.Atoi(datePart); err == nil {
				if parts > latest {
					latest = parts
				}
			}
		}
	}
	return latest, nil
}

func listCIKernels(arch string) ([]KernelInfo, error) {
	buildID, err := listLatestCIBuild()
	if err != nil {
		return nil, fmt.Errorf("failed to find CI build: %w", err)
	}

	prefix := fmt.Sprintf("%s%s/%s/vmlinux-", ciPrefix, buildID, arch)
	url := fmt.Sprintf("%s/?list-type=2&prefix=%s", s3BaseURL, prefix)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result s3ListResult
	if err := xml.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	var kernels []KernelInfo
	for _, c := range result.Contents {
		if strings.HasSuffix(c.Key, ".config") {
			continue
		}
		name := filepath.Base(c.Key)
		version := strings.TrimPrefix(name, "vmlinux-")

		kernels = append(kernels, KernelInfo{
			Name:    name,
			Key:     c.Key,
			Size:    c.Size,
			ModTime: c.LastModified[:10],
			Source:  "ci",
			Version: version,
			Arch:    arch,
		})
	}

	sort.Slice(kernels, func(i, j int) bool {
		return kernels[i].Version < kernels[j].Version
	})

	return kernels, nil
}

func listAllKernelsForArch(arch string) ([]KernelInfo, error) {
	var all []KernelInfo

	qs, err := listQuickstartKernels(arch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to list quickstart kernels for %s: %v\n", arch, err)
	} else {
		all = append(all, qs...)
	}

	ci, err := listCIKernels(arch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to list CI kernels for %s: %v\n", arch, err)
	} else {
		all = append(all, ci...)
	}

	return all, nil
}

func formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func parseSelection(input string, max int) ([]int, error) {
	var selected []int
	parts := strings.Split(input, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if strings.Contains(part, "-") {
			rangeParts := strings.SplitN(part, "-", 2)
			start, err := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
			if err != nil {
				return nil, fmt.Errorf("invalid range start: %s", rangeParts[0])
			}
			end, err := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid range end: %s", rangeParts[1])
			}
			if start < 1 || end > max || start > end {
				return nil, fmt.Errorf("range %d-%d out of bounds (1-%d)", start, end, max)
			}
			for i := start; i <= end; i++ {
				selected = append(selected, i)
			}
		} else {
			num, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid number: %s", part)
			}
			if num < 1 || num > max {
				return nil, fmt.Errorf("number %d out of bounds (1-%d)", num, max)
			}
			selected = append(selected, num)
		}
	}

	seen := make(map[int]bool)
	var unique []int
	for _, n := range selected {
		if !seen[n] {
			seen[n] = true
			unique = append(unique, n)
		}
	}

	return unique, nil
}

func printKernelList(kernels []KernelInfo) {
	fmt.Println("┌──────┬──────┬──────────────────────────────────┬───────────┬──────────────┬────────┐")
	fmt.Println("│ Num  │ Src  │ Name                             │ Size      │ Modified     │ Arch   │")
	fmt.Println("├──────┼──────┼──────────────────────────────────┼───────────┼──────────────┼────────┤")
	for i, k := range kernels {
		src := "QS"
		if k.Source == "ci" {
			src = "CI"
		}
		fmt.Printf("│ %4d │ %-4s │ %-32s │ %9s │ %12s │ %-6s │\n",
			i+1, src, k.Name, formatSize(k.Size), k.ModTime, k.Arch)
	}
	fmt.Println("└──────┴──────┴──────────────────────────────────┴───────────┴──────────────┴────────┘")
	fmt.Println()
	fmt.Println(i18n.T("vmm.download.legend"))
}

func downloadKernel(k KernelInfo) error {
	kernelDir := filepath.Join("kernel", k.Arch)
	if err := os.MkdirAll(kernelDir, 0755); err != nil {
		return fmt.Errorf("failed to create kernel directory: %w", err)
	}

	destPath := filepath.Join(kernelDir, k.Name)

	if _, err := os.Stat(destPath); err == nil {
		fmt.Printf("%s\n", i18n.Tf("vmm.download.skip", k.Name))
		return nil
	}

	downloadURL := fmt.Sprintf("%s/%s", s3BaseURL, k.Key)
	fmt.Printf("%s\n", i18n.Tf("vmm.download.progress", k.Name, formatSize(k.Size)))

	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("download failed for %s: %w", k.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed for %s: HTTP %d", k.Name, resp.StatusCode)
	}

	tmpPath := destPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}

	pw := &progressWriter{total: resp.ContentLength}
	_, err = io.Copy(out, io.TeeReader(resp.Body, pw))
	out.Close()
	fmt.Println()

	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("download failed: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename file: %w", err)
	}

	fmt.Printf("%s\n", i18n.Tf("vmm.download.success", k.Name, destPath))
	return nil
}

func DownloadKernel(downloadAll bool, arch string) error {
	var archs []string
	if arch == "all" || arch == "" {
		archs = supportedArchs
	} else {
		archs = []string{arch}
	}

	fmt.Println(i18n.T("vmm.download.fetching"))
	fmt.Printf("%s\n\n", i18n.Tf("vmm.download.table_header", strings.Join(archs, ", ")))

	var allKernels []KernelInfo
	for _, a := range archs {
		kernels, err := listAllKernelsForArch(a)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to list kernels for %s: %v\n", a, err)
			continue
		}
		allKernels = append(allKernels, kernels...)
	}

	if len(allKernels) == 0 {
		return fmt.Errorf("%s", i18n.T("vmm.download.no_kernels"))
	}

	printKernelList(allKernels)

	if downloadAll {
		fmt.Printf("%s\n\n", i18n.Tf("vmm.download.downloading_all", len(allKernels)))
		var failed int
		for _, k := range allKernels {
			if err := downloadKernel(k); err != nil {
				fmt.Printf("%s\n", i18n.Tf("vmm.download.error", err))
				failed++
			}
		}
		fmt.Printf("\n%s\n", i18n.Tf("vmm.download.done", len(allKernels)-failed, len(allKernels)))
		return nil
	}

	fmt.Print(i18n.T("vmm.download.prompt"))
	var input string
	fmt.Scan(&input)

	input = strings.TrimSpace(input)
	if input == "" || input == "0" {
		fmt.Println(i18n.T("vmm.download.cancelled"))
		return nil
	}

	var toDownload []int
	var err error
	if input == "all" || input == "a" {
		for i := range allKernels {
			toDownload = append(toDownload, i+1)
		}
	} else {
		toDownload, err = parseSelection(input, len(allKernels))
		if err != nil {
			return fmt.Errorf("invalid selection: %w", err)
		}
	}

	fmt.Printf("%s\n\n", i18n.Tf("vmm.download.downloading", len(toDownload)))
	var failed int
	for _, idx := range toDownload {
		if err := downloadKernel(allKernels[idx-1]); err != nil {
			fmt.Printf("%s\n", i18n.Tf("vmm.download.error", err))
			failed++
		}
	}

	fmt.Printf("\n%s\n", i18n.Tf("vmm.download.done", len(toDownload)-failed, len(toDownload)))

	if failed == 0 && len(toDownload) > 0 {
		fmt.Printf("\n")
		for _, idx := range toDownload {
			k := allKernels[idx-1]
			fmt.Printf("  sudo ./litevm vmm --kernel kernel/%s/%s --rootfs rootfs/rootfs.ext4\n", k.Arch, k.Name)
		}
	}

	return nil
}
