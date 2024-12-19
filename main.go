package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

type ColorSets struct {
	Light map[string]string `json:"light"`
	Dark  map[string]string `json:"dark"`
}

type ContrastResult struct {
	ForegroundHex  string  `json:"foregroundHex"`
	ForegroundName string  `json:"foregroundName"`
	BackgroundHex  string  `json:"backgroundHex"`
	BackgroundName string  `json:"backgroundName"`
	ContrastRatio  float64 `json:"contrastRatio"`
	LevelSmallText string  `json:"levelSmallText"`
	LevelLargeText string  `json:"levelLargeText"`
	RequiresFix    bool    `json:"requiresFix"`
}

type WCAGLevels struct {
	AAA   []ContrastResult `json:"AAA"`
	AA    []ContrastResult `json:"AA"`
	Other []ContrastResult `json:"Other"`
}

func LoadColors(filename string) (*ColorSets, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var colors ColorSets
	err = json.Unmarshal(data, &colors)
	if err != nil {
		return nil, err
	}
	return &colors, nil
}

func toLinear(value float64) float64 {
	if value <= 0.03928 {
		return value / 12.92
	}
	return math.Pow((value+0.055)/1.055, 2.4)
}

func relativeLuminance(hex string) (float64, error) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return 0, fmt.Errorf("invalid hex: %s", hex)
	}
	r, err := parseHex(hex[0:2])
	if err != nil {
		return 0, err
	}
	g, err := parseHex(hex[2:4])
	if err != nil {
		return 0, err
	}
	b, err := parseHex(hex[4:6])
	if err != nil {
		return 0, err
	}

	R := toLinear(float64(r) / 255.0)
	G := toLinear(float64(g) / 255.0)
	B := toLinear(float64(b) / 255.0)

	return 0.2126*R + 0.7152*G + 0.0722*B, nil
}

func parseHex(h string) (int64, error) {
	return strconv.ParseInt(h, 16, 64)
}

func contrastRatio(fgHex, bgHex string) (float64, error) {
	fgLum, err := relativeLuminance(fgHex)
	if err != nil {
		return 0, err
	}
	bgLum, err := relativeLuminance(bgHex)
	if err != nil {
		return 0, err
	}
	L1 := math.Max(fgLum, bgLum)
	L2 := math.Min(fgLum, bgLum)
	return (L1 + 0.05) / (L2 + 0.05), nil
}

func complianceLevel(ratio float64) string {
	if ratio >= 7 {
		return "AAA"
	} else if ratio >= 4.5 {
		return "AA"
	} else {
		return "Fail"
	}
}

func complianceLevelLarge(ratio float64) string {
	if ratio >= 4.5 {
		return "AAA"
	} else if ratio >= 3 {
		return "AA"
	} else {
		return "Fail"
	}
}

func allContrastsHandler(w http.ResponseWriter, r *http.Request) {
	colors, err := LoadColors("colors.json")
	if err != nil {
		http.Error(w, "Failed to load colors: "+err.Error(), http.StatusInternalServerError)
		return
	}

	search := strings.ToLower(r.URL.Query().Get("search"))

	results := WCAGLevels{
		AAA:   []ContrastResult{},
		AA:    []ContrastResult{},
		Other: []ContrastResult{},
	}

	lightNames := make([]string, 0, len(colors.Light))
	for name := range colors.Light {
		lightNames = append(lightNames, name)
	}
	sort.Strings(lightNames)

	darkNames := make([]string, 0, len(colors.Dark))
	for name := range colors.Dark {
		darkNames = append(darkNames, name)
	}
	sort.Strings(darkNames)

	for _, nameLight := range lightNames {
		fgHex := colors.Light[nameLight]
		for _, nameDark := range darkNames {
			bgHex := colors.Dark[nameDark]

			if search != "" {
				if !strings.Contains(strings.ToLower(nameLight), search) && !strings.Contains(strings.ToLower(nameDark), search) {
					continue
				}
			}

			ratio, err := contrastRatio(fgHex, bgHex)
			if err != nil {
				log.Printf("Error calculating contrast for %s on %s: %v", fgHex, bgHex, err)
				continue
			}

			levelSmall := complianceLevel(ratio)
			levelLarge := complianceLevelLarge(ratio)

			requiresFix := false
			if levelSmall == "Fail" || levelLarge == "Fail" {
				requiresFix = true
			}

			result := ContrastResult{
				ForegroundHex:  fgHex,
				ForegroundName: nameLight,
				BackgroundHex:  bgHex,
				BackgroundName: nameDark,
				ContrastRatio:  math.Round(ratio*100) / 100,
				LevelSmallText: levelSmall,
				LevelLargeText: levelLarge,
				RequiresFix:    requiresFix,
			}

			switch levelSmall {
			case "AAA":
				results.AAA = append(results.AAA, result)
			case "AA":
				results.AA = append(results.AA, result)
			default:
				results.Other = append(results.Other, result)
			}
		}
	}

	tmpl, err := template.New("results").Parse(htmlTemplate)
	if err != nil {
		http.Error(w, "Failed to parse template: "+err.Error(), http.StatusInternalServerError)
		return
	}

	data := struct {
		AAA    []ContrastResult
		AA     []ContrastResult
		Other  []ContrastResult
		Search string
	}{
		AAA:    results.AAA,
		AA:     results.AA,
		Other:  results.Other,
		Search: r.URL.Query().Get("search"),
	}

	w.Header().Set("Content-Type", "text/html")
	err = tmpl.Execute(w, data)
	if err != nil {
		http.Error(w, "Failed to execute template: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

func downloadHandler(w http.ResponseWriter, r *http.Request) {
	colors, err := LoadColors("colors.json")
	if err != nil {
		http.Error(w, "Failed to load colors: "+err.Error(), http.StatusInternalServerError)
		return
	}

	results := WCAGLevels{
		AAA:   []ContrastResult{},
		AA:    []ContrastResult{},
		Other: []ContrastResult{},
	}

	lightNames := make([]string, 0, len(colors.Light))
	for name := range colors.Light {
		lightNames = append(lightNames, name)
	}
	sort.Strings(lightNames)

	darkNames := make([]string, 0, len(colors.Dark))
	for name := range colors.Dark {
		darkNames = append(darkNames, name)
	}
	sort.Strings(darkNames)

	for _, nameLight := range lightNames {
		fgHex := colors.Light[nameLight]
		for _, nameDark := range darkNames {
			bgHex := colors.Dark[nameDark]

			ratio, err := contrastRatio(fgHex, bgHex)
			if err != nil {
				log.Printf("Error calculating contrast for %s on %s: %v", fgHex, bgHex, err)
				continue
			}

			levelSmall := complianceLevel(ratio)
			levelLarge := complianceLevelLarge(ratio)

			requiresFix := false
			if levelSmall == "Fail" || levelLarge == "Fail" {
				requiresFix = true
			}

			result := ContrastResult{
				ForegroundHex:  fgHex,
				ForegroundName: nameLight,
				BackgroundHex:  bgHex,
				BackgroundName: nameDark,
				ContrastRatio:  math.Round(ratio*100) / 100,
				LevelSmallText: levelSmall,
				LevelLargeText: levelLarge,
				RequiresFix:    requiresFix,
			}

			switch levelSmall {
			case "AAA":
				results.AAA = append(results.AAA, result)
			case "AA":
				results.AA = append(results.AA, result)
			default:
				results.Other = append(results.Other, result)
			}
		}
	}

	file, err := os.Create("contrast_results.csv")
	if err != nil {
		http.Error(w, "Failed to create CSV file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	writer.Write([]string{
		"Foreground Name",
		"Foreground Hex",
		"Background Name",
		"Background Hex",
		"Contrast Ratio",
		"WCAG Level (Small Text)",
		"WCAG Level (Large Text)",
		"Requires Fix",
	})

	writeResultsToCSV(writer, results.AAA, "AAA")
	writeResultsToCSV(writer, results.AA, "AA")
	writeResultsToCSV(writer, results.Other, "Other")

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment;filename=contrast_results.csv")
	http.ServeFile(w, r, "contrast_results.csv")
}

func writeResultsToCSV(writer *csv.Writer, results []ContrastResult, category string) {
	for _, result := range results {
		writer.Write([]string{
			result.ForegroundName,
			result.ForegroundHex,
			result.BackgroundName,
			result.BackgroundHex,
			strconv.FormatFloat(result.ContrastRatio, 'f', 2, 64),
			result.LevelSmallText,
			result.LevelLargeText,
			strconv.FormatBool(result.RequiresFix),
		})
	}
}

func main() {
	if _, err := ioutil.ReadFile("colors.json"); err != nil {
		log.Fatalf("colors.json file not found. Please ensure it exists in the current directory.")
	}

	http.HandleFunc("/", allContrastsHandler)
	http.HandleFunc("/download", downloadHandler)

	go func() {
		fmt.Println("Server is running on http://localhost:8080/")
		if err := http.ListenAndServe(":8080", nil); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	openBrowser("http://localhost:8080/")

	select {}
}

func openBrowser(url string) {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}

	err := exec.Command(cmd, args...).Start()
	if err != nil {
		log.Printf("Failed to open browser: %v", err)
	}
}

const htmlTemplate = `
<!DOCTYPE html>
<html lang="ja">
<head>
    <meta charset="UTF-8">
    <title>Contrast Checker Results / コントラストチェッカー結果</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            background-color: #f4f4f4;
            color: #333;
            margin: 0;
            padding: 20px;
            transition: background-color 0.3s, color 0.3s;
        }
        .dark body {
            background-color: #1e1e1e;
            color: #f4f4f4;
        }
        h1, h2 {
            color: #444;
        }
        .dark h1, .dark h2 {
            color: #ddd;
        }
        .container {
            max-width: 1200px;
            margin: auto;
        }
        .language-toggle {
            position: fixed;
            top: 20px;
            left: 20px;
            padding: 10px 20px;
            background-color: #555;
            color: #fff;
            border: none;
            border-radius: 4px;
            cursor: pointer;
            transition: background-color 0.3s, transform 0.3s;
            z-index: 1001;
        }
        .language-toggle:hover {
            background-color: #333;
            transform: scale(1.05);
        }
        .category {
            margin-bottom: 40px;
        }
        .category h2 {
            border-bottom: 2px solid #ccc;
            padding-bottom: 10px;
        }
        .dark .category h2 {
            border-bottom: 2px solid #555;
        }
        .color-pair {
            display: flex;
            align-items: center;
            background-color: #fff;
            padding: 15px;
            margin-bottom: 15px;
            border-radius: 8px;
            box-shadow: 0 2px 5px rgba(0,0,0,0.1);
            transition: background-color 0.3s, box-shadow 0.3s;
        }
        .dark .color-pair {
            background-color: #2c2c2c;
            box-shadow: 0 2px 5px rgba(255,255,255,0.1);
        }
        .color-pair:hover {
            box-shadow: 0 4px 10px rgba(0,0,0,0.2);
        }
        .dark .color-pair:hover {
            box-shadow: 0 4px 10px rgba(255,255,255,0.2);
        }
        .color-box {
            width: 60px;
            height: 60px;
            border: 1px solid #ccc;
            border-radius: 4px;
            margin-right: 20px;
            display: flex;
            align-items: center;
            justify-content: center;
            color: #fff;
            font-weight: bold;
            text-shadow: 0 1px 2px rgba(0,0,0,0.5);
            cursor: pointer;
            transition: border 0.3s;
        }
        .dark .color-box {
            border: 1px solid #555;
            text-shadow: none;
        }
        .contrast-info {
            flex-grow: 1;
        }
        .fail {
            border-left: 5px solid #e74c3c;
            background-color: #fdecea;
            transition: background-color 0.3s, border-color 0.3s;
        }
        .dark .fail {
            background-color: #5a1a1a;
        }
        .summary {
            margin-bottom: 20px;
            padding: 15px;
            background-color: #fff;
            border-radius: 8px;
            box-shadow: 0 2px 5px rgba(0,0,0,0.1);
            transition: background-color 0.3s, box-shadow 0.3s;
        }
        .dark .summary {
            background-color: #2c2c2c;
            box-shadow: 0 2px 5px rgba(255,255,255,0.1);
        }
        .search-bar {
            margin-bottom: 20px;
        }
        .dark .search-bar input {
            background-color: #3a3a3a;
            color: #f4f4f4;
            border: 1px solid #555;
        }
        .download-button {
            margin-bottom: 20px;
        }
        .download-button a {
            display: inline-block;
            font-size: 16px;
            border: 2px solid #555;
            transition: background-color 0.3s, color 0.3s, border-color 0.3s, transform 0.3s;
            padding: 10px 20px;
            border-radius: 4px;
            background-color: #555;
            color: #fff;
            text-decoration: none;
        }
        .download-button a:hover {
            background-color: #fff;
            color: #555;
            border-color: #333;
            transform: translateY(-2px);
        }
        .theme-toggle {
            position: fixed;
            top: 20px;
            right: 20px;
            padding: 10px;
            background-color: #555;
            color: #fff;
            border: none;
            border-radius: 50%;
            cursor: pointer;
            transition: background-color 0.3s, transform 0.3s;
            z-index: 1001;
        }
        .theme-toggle:hover {
            background-color: #333;
            transform: scale(1.1);
        }
        @media (max-width: 768px) {
            .color-pair {
                flex-direction: column;
                align-items: flex-start;
            }
            .color-box {
                margin-right: 0;
                margin-bottom: 10px;
            }
        }
        .color-picker {
            margin-bottom: 20px;
        }
        .color-picker label {
            margin-right: 10px;
        }
        .color-picker input[type="color"] {
            margin-right: 20px;
            border: none;
            width: 40px;
            height: 40px;
            padding: 0;
        }
        .color-picker p {
            font-size: 16px;
            font-weight: bold;
        }
        .dark .color-picker p {
            color: #f4f4f4;
        }
        #modal {
            display: none;
            position: fixed;
            top: 0;
            left: 0;
            width: 100%;
            height: 100%;
            background-color: rgba(0,0,0,0.5);
            justify-content: center;
            align-items: center;
            z-index: 1000;
            opacity: 0;
            transition: opacity 0.3s ease-in-out;
        }
        #modal.show {
            display: flex;
            opacity: 1;
        }
        #modal-content {
            background-color: #fff;
            padding: 20px;
            border-radius: 8px;
            max-width: 600px;
            width: 90%;
            box-shadow: 0 4px 10px rgba(0,0,0,0.2);
            transition: transform 0.3s ease-in-out, opacity 0.3s ease-in-out;
        }
        .dark #modal-content {
            background-color: #2c2c2c;
            color: #f4f4f4;
        }
        #close-modal {
            float: right;
            font-size: 24px;
            font-weight: bold;
            cursor: pointer;
            background: none;
            border: none;
            color: inherit;
        }
        .modal-list {
            max-height: 70vh;
            overflow-y: auto;
        }
        .modal-list .color-pair {
            margin-bottom: 20px;
        }
        @keyframes fadeInScale {
            from {
                opacity: 0;
                transform: scale(0.9);
            }
            to {
                opacity: 1;
                transform: scale(1);
            }
        }
        @keyframes fadeOutScale {
            from {
                opacity: 1;
                transform: scale(1);
            }
            to {
                opacity: 0;
                transform: scale(0.9);
            }
        }
        .visually-hidden {
            position: absolute;
            width: 1px;
            height: 1px;
            padding: 0;
            margin: -1px;
            overflow: hidden;
            clip: rect(0, 0, 0, 0);
            border: 0;
        }
        .color-pair:focus-within {
            outline: 2px solid #3498db;
        }
        #show-modal-btn:focus, #close-modal:focus {
            outline: 2px solid #3498db;
        }
        #show-modal-btn:hover, #show-modal-btn:focus {
            background-color: #c0392b;
            border-color: #c0392b;
        }
        #close-modal:hover, #close-modal:focus {
            color: #e74c3c;
        }
    </style>
</head>
<body>
    <button class="language-toggle" id="language-toggle" aria-label="Switch Language">EN / JP</button>
    <button class="theme-toggle" id="theme-toggle" aria-label="Toggle Dark Mode">🌓</button>
    <div class="container">
        <h1 class="lang" data-lang="en">Contrast Checker Results</h1>
        <h1 class="lang" data-lang="jp" style="display:none;">コントラストチェッカー結果</h1>

        <div class="summary" aria-live="polite">
            <p class="lang" data-lang="en"><strong>AAA Compliance:</strong> {{len .AAA}} results</p>
            <p class="lang" data-lang="jp" style="display:none;"><strong>AAA適合:</strong> {{len .AAA}} 件</p>
            <p class="lang" data-lang="en"><strong>AA Compliance:</strong> {{len .AA}} results</p>
            <p class="lang" data-lang="jp" style="display:none;"><strong>AA適合:</strong> {{len .AA}} 件</p>
            <p class="lang" data-lang="en"><strong>Others:</strong> {{len .Other}} results</p>
            <p class="lang" data-lang="jp" style="display:none;"><strong>それ以外:</strong> {{len .Other}} 件</p>
        </div>

        <div class="search-bar">
            <label for="search-input" class="visually-hidden" data-lang="en">Search by Color Name</label>
            <label for="search-input" class="visually-hidden" data-lang="jp" style="display:none;">色名で検索</label>
            <input type="text" id="search-input" placeholder="Search by Color Name..." value="{{.Search}}" style="padding: 8px; width: 200px; border: 1px solid #ccc; border-radius: 4px;" aria-label="Search by Color Name">
        </div>

        <div class="download-button" style="margin-bottom: 20px;">
            <a href="/download" aria-label="Download Results as CSV" class="lang" data-lang="en">Download Results as CSV</a>
            <a href="/download" aria-label="結果をCSVでダウンロード" class="lang" data-lang="jp" style="display:none;">結果をCSVでダウンロード</a>
        </div>

        {{if .Other}}
        <div class="show-modal-button" style="margin-bottom: 20px;">
            <button id="show-modal-btn" style="padding: 10px 20px; border: 2px solid #e74c3c; background-color: #e74c3c; color: #fff; border-radius: 4px; cursor: pointer;" aria-haspopup="dialog" aria-controls="modal">
                <span class="lang" data-lang="en">Show Fixable Combinations</span>
                <span class="lang" data-lang="jp" style="display:none;">修正が必要な色の組み合わせを表示</span>
            </button>
        </div>
        {{end}}

        {{if .AAA}}
        <div class="category AAA" aria-labelledby="AAA Compliance">
            <h2 class="lang" data-lang="en" id="AAA Compliance">AAA Compliance</h2>
            <h2 class="lang" data-lang="jp" id="AAA Compliance" style="display:none;">AAA適合</h2>
            {{range .AAA}}
            <div class="color-pair" tabindex="0">
                <button class="color-box" style="background-color: {{.ForegroundHex}};" aria-label="Foreground Color {{.ForegroundName}} ({{.ForegroundHex}})">
                    FG
                </button>
                <button class="color-box" style="background-color: {{.BackgroundHex}};" aria-label="Background Color {{.BackgroundName}} ({{.BackgroundHex}})">
                    BG
                </button>
                <div class="contrast-info">
                    <p class="lang" data-lang="en"><strong>Foreground:</strong> <span class="foreground-name">{{.ForegroundName}}</span> ({{.ForegroundHex}})</p>
                    <p class="lang" data-lang="jp" style="display:none;"><strong>Foreground:</strong> <span class="foreground-name">{{.ForegroundName}}</span> ({{.ForegroundHex}})</p>
                    <p class="lang" data-lang="en"><strong>Background:</strong> <span class="background-name">{{.BackgroundName}}</span> ({{.BackgroundHex}})</p>
                    <p class="lang" data-lang="jp" style="display:none;"><strong>Background:</strong> <span class="background-name">{{.BackgroundName}}</span> ({{.BackgroundHex}})</p>
                    <p class="lang" data-lang="en"><strong>Contrast Ratio:</strong> {{.ContrastRatio}}</p>
                    <p class="lang" data-lang="jp" style="display:none;"><strong>コントラスト比:</strong> {{.ContrastRatio}}</p>
                    <p class="lang" data-lang="en"><strong>WCAG Level (Small Text):</strong> {{.LevelSmallText}}</p>
                    <p class="lang" data-lang="jp" style="display:none;"><strong>WCAGレベル (小文字):</strong> {{.LevelSmallText}}</p>
                    <p class="lang" data-lang="en"><strong>WCAG Level (Large Text):</strong> {{.LevelLargeText}}</p>
                    <p class="lang" data-lang="jp" style="display:none;"><strong>WCAGレベル (大文字):</strong> {{.LevelLargeText}}</p>
                </div>
            </div>
            {{end}}
        </div>
        {{end}}

        {{if .AA}}
        <div class="category AA" aria-labelledby="AA Compliance">
            <h2 class="lang" data-lang="en" id="AA Compliance">AA Compliance</h2>
            <h2 class="lang" data-lang="jp" id="AA Compliance" style="display:none;">AA適合</h2>
            {{range .AA}}
            <div class="color-pair" tabindex="0">
                <button class="color-box" style="background-color: {{.ForegroundHex}};" aria-label="Foreground Color {{.ForegroundName}} ({{.ForegroundHex}})">
                    FG
                </button>
                <button class="color-box" style="background-color: {{.BackgroundHex}};" aria-label="Background Color {{.BackgroundName}} ({{.BackgroundHex}})">
                    BG
                </button>
                <div class="contrast-info">
                    <p class="lang" data-lang="en"><strong>Foreground:</strong> <span class="foreground-name">{{.ForegroundName}}</span> ({{.ForegroundHex}})</p>
                    <p class="lang" data-lang="jp" style="display:none;"><strong>Foreground:</strong> <span class="foreground-name">{{.ForegroundName}}</span> ({{.ForegroundHex}})</p>
                    <p class="lang" data-lang="en"><strong>Background:</strong> <span class="background-name">{{.BackgroundName}}</span> ({{.BackgroundHex}})</p>
                    <p class="lang" data-lang="jp" style="display:none;"><strong>Background:</strong> <span class="background-name">{{.BackgroundName}}</span> ({{.BackgroundHex}})</p>
                    <p class="lang" data-lang="en"><strong>Contrast Ratio:</strong> {{.ContrastRatio}}</p>
                    <p class="lang" data-lang="jp" style="display:none;"><strong>コントラスト比:</strong> {{.ContrastRatio}}</p>
                    <p class="lang" data-lang="en"><strong>WCAG Level (Small Text):</strong> {{.LevelSmallText}}</p>
                    <p class="lang" data-lang="jp" style="display:none;"><strong>WCAGレベル (小文字):</strong> {{.LevelSmallText}}</p>
                    <p class="lang" data-lang="en"><strong>WCAG Level (Large Text):</strong> {{.LevelLargeText}}</p>
                    <p class="lang" data-lang="jp" style="display:none;"><strong>WCAGレベル (大文字):</strong> {{.LevelLargeText}}</p>
                </div>
            </div>
            {{end}}
        </div>
        {{end}}

        {{if .Other}}
        <div class="category Other" aria-labelledby="Other Compliance">
            <h2 class="lang" data-lang="en" id="Other Compliance">Others</h2>
            <h2 class="lang" data-lang="jp" id="Other Compliance" style="display:none;">それ以外</h2>
            {{range .Other}}
            <div class="color-pair fail" tabindex="0">
                <button class="color-box" style="background-color: {{.ForegroundHex}};" aria-label="Foreground Color {{.ForegroundName}} ({{.ForegroundHex}})">
                    FG
                </button>
                <button class="color-box" style="background-color: {{.BackgroundHex}};" aria-label="Background Color {{.BackgroundName}} ({{.BackgroundHex}})">
                    BG
                </button>
                <div class="contrast-info">
                    <p class="lang" data-lang="en"><strong>Foreground:</strong> <span class="foreground-name">{{.ForegroundName}}</span> ({{.ForegroundHex}})</p>
                    <p class="lang" data-lang="jp" style="display:none;"><strong>Foreground:</strong> <span class="foreground-name">{{.ForegroundName}}</span> ({{.ForegroundHex}})</p>
                    <p class="lang" data-lang="en"><strong>Background:</strong> <span class="background-name">{{.BackgroundName}}</span> ({{.BackgroundHex}})</p>
                    <p class="lang" data-lang="jp" style="display:none;"><strong>Background:</strong> <span class="background-name">{{.BackgroundName}}</span> ({{.BackgroundHex}})</p>
                    <p class="lang" data-lang="en"><strong>Contrast Ratio:</strong> {{.ContrastRatio}}</p>
                    <p class="lang" data-lang="jp" style="display:none;"><strong>コントラスト比:</strong> {{.ContrastRatio}}</p>
                    <p class="lang" data-lang="en"><strong>WCAG Level (Small Text):</strong> {{.LevelSmallText}}</p>
                    <p class="lang" data-lang="jp" style="display:none;"><strong>WCAGレベル (小文字):</strong> {{.LevelSmallText}}</p>
                    <p class="lang" data-lang="en"><strong>WCAG Level (Large Text):</strong> {{.LevelLargeText}}</p>
                    <p class="lang" data-lang="jp" style="display:none;"><strong>WCAGレベル (大文字):</strong> {{.LevelLargeText}}</p>
                    <p class="lang" data-lang="en"><strong>Action Required:</strong> Fix the color combination.</p>
                    <p class="lang" data-lang="jp" style="display:none;"><strong>必要なアクション:</strong> 色の組み合わせを修正してください。</p>
                </div>
            </div>
            {{end}}
        </div>
        {{end}}

        <div class="color-picker">
            <label for="foreground-color" class="visually-hidden" data-lang="en">Foreground Color</label>
            <label for="foreground-color" class="visually-hidden" data-lang="jp" style="display:none;">前景色</label>
            <input type="color" id="foreground-color" name="foreground-color" value="#000000" aria-label="Foreground Color">

            <label for="background-color" class="visually-hidden" data-lang="en">Background Color</label>
            <label for="background-color" class="visually-hidden" data-lang="jp" style="display:none;">背景色</label>
            <input type="color" id="background-color" name="background-color" value="#ffffff" aria-label="Background Color">

            <p class="lang" data-lang="en">Contrast Ratio: <span id="contrast-ratio">0</span></p>
            <p class="lang" data-lang="jp" style="display:none;">コントラスト比: <span id="contrast-ratio">0</span></p>
        </div>
    </div>

    <div id="modal" role="dialog" aria-labelledby="modal-title" aria-modal="true">
        <div id="modal-content" role="document">
            <button id="close-modal" aria-label="Close Modal">&times;</button>
            <h2 class="lang" data-lang="en" id="modal-title">Fixable Color Combinations</h2>
            <h2 class="lang" data-lang="jp" id="modal-title" style="display:none;">修正が必要な色の組み合わせ</h2>
            <div class="modal-list">
                {{if .Other}}
                    {{range .Other}}
                    <div class="color-pair fail" tabindex="0">
                        <button class="color-box" style="background-color: {{.ForegroundHex}};" aria-label="Foreground Color {{.ForegroundName}} ({{.ForegroundHex}})">
                            FG
                        </button>
                        <button class="color-box" style="background-color: {{.BackgroundHex}};" aria-label="Background Color {{.BackgroundName}} ({{.BackgroundHex}})">
                            BG
                        </button>
                        <div class="contrast-info">
                            <p class="lang" data-lang="en"><strong>Foreground:</strong> {{.ForegroundName}} ({{.ForegroundHex}})</p>
                            <p class="lang" data-lang="jp" style="display:none;"><strong>Foreground:</strong> {{.ForegroundName}} ({{.ForegroundHex}})</p>
                            <p class="lang" data-lang="en"><strong>Background:</strong> {{.BackgroundName}} ({{.BackgroundHex}})</p>
                            <p class="lang" data-lang="jp" style="display:none;"><strong>Background:</strong> {{.BackgroundName}} ({{.BackgroundHex}})</p>
                            <p class="lang" data-lang="en"><strong>Contrast Ratio:</strong> {{.ContrastRatio}}</p>
                            <p class="lang" data-lang="jp" style="display:none;"><strong>コントラスト比:</strong> {{.ContrastRatio}}</p>
                            <p class="lang" data-lang="en"><strong>WCAG Level (Small Text):</strong> {{.LevelSmallText}}</p>
                            <p class="lang" data-lang="jp" style="display:none;"><strong>WCAGレベル (小文字):</strong> {{.LevelSmallText}}</p>
                            <p class="lang" data-lang="en"><strong>WCAG Level (Large Text):</strong> {{.LevelLargeText}}</p>
                            <p class="lang" data-lang="jp" style="display:none;"><strong>WCAGレベル (大文字):</strong> {{.LevelLargeText}}</p>
                            <p class="lang" data-lang="en"><strong>Action Required:</strong> Fix the color combination.</p>
                            <p class="lang" data-lang="jp" style="display:none;"><strong>必要なアクション:</strong> 色の組み合わせを修正してください。</p>
                            <div style="margin-top:10px;">
                                <div style="display: flex; align-items: center;">
                                    <div style="width: 30px; height: 30px; background-color: {{.ForegroundHex}}; margin-right: 10px; border: 1px solid #ccc;"></div>
                                    <div style="width: 30px; height: 30px; background-color: {{.BackgroundHex}}; border: 1px solid #ccc;"></div>
                                </div>
                            </div>
                        </div>
                    </div>
                    {{end}}
                {{else}}
                    <p class="lang" data-lang="en">No fixable color combinations found.</p>
                    <p class="lang" data-lang="jp" style="display:none;">修正が必要な色の組み合わせはありません。</p>
                {{end}}
            </div>
        </div>
    </div>

    <script>
        const themeToggleBtn = document.getElementById('theme-toggle');
        const languageToggleBtn = document.getElementById('language-toggle');
        const currentTheme = localStorage.getItem('theme') ? localStorage.getItem('theme') : null;
        const currentLanguage = localStorage.getItem('language') ? localStorage.getItem('language') : 'en';

        if (currentTheme) {
            document.documentElement.classList.add(currentTheme);
            if (currentTheme === 'dark') {
                themeToggleBtn.textContent = '☀️';
            } else {
                themeToggleBtn.textContent = '🌙';
            }
        }

        if (currentLanguage === 'jp') {
            switchLanguage('jp');
        } else {
            switchLanguage('en');
        }

        themeToggleBtn.addEventListener('click', () => {
            document.documentElement.classList.toggle('dark');
            let theme = 'light';
            if (document.documentElement.classList.contains('dark')) {
                theme = 'dark';
                themeToggleBtn.textContent = '☀️';
            } else {
                theme = 'light';
                themeToggleBtn.textContent = '🌙';
            }
            localStorage.setItem('theme', theme);
        });

        languageToggleBtn.addEventListener('click', () => {
            const newLanguage = localStorage.getItem('language') === 'en' ? 'jp' : 'en';
            switchLanguage(newLanguage);
            localStorage.setItem('language', newLanguage);
        });

        function switchLanguage(lang) {
            const elements = document.querySelectorAll('.lang');
            elements.forEach(el => {
                if (el.getAttribute('data-lang') === lang) {
                    el.style.display = '';
                } else {
                    el.style.display = 'none';
                }
            });
        }

        const searchInput = document.getElementById('search-input');

        searchInput.addEventListener('input', function() {
            const query = this.value.toLowerCase();
            const colorPairs = document.querySelectorAll('.color-pair');

            colorPairs.forEach(pair => {
                const fgName = pair.querySelector('.foreground-name').innerText.toLowerCase();
                const bgName = pair.querySelector('.background-name').innerText.toLowerCase();

                if (fgName.includes(query) || bgName.includes(query)) {
                    pair.style.display = 'flex';
                } else {
                    pair.style.display = 'none';
                }
            });
        });

        const modal = document.getElementById('modal');
        const closeModal = document.getElementById('close-modal');
        const showModalBtn = document.getElementById('show-modal-btn');

        if (showModalBtn) {
            showModalBtn.addEventListener('click', () => {
                modal.classList.add('show');
                closeModal.focus();
            });
        }

        closeModal.addEventListener('click', () => {
            modal.classList.remove('show');
            if (showModalBtn) {
                showModalBtn.focus();
            }
        });

        window.addEventListener('click', (e) => {
            if (e.target == modal) {
                modal.classList.remove('show');
                if (showModalBtn) {
                    showModalBtn.focus();
                }
            }
        });

        document.addEventListener('keydown', function(event) {
            if (event.key === 'Escape') {
                modal.classList.remove('show');
                if (showModalBtn) {
                    showModalBtn.focus();
                }
            }

            if (modal.classList.contains('show')) {
                const focusableElements = modal.querySelectorAll('button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])');
                const firstElement = focusableElements[0];
                const lastElement = focusableElements[focusableElements.length - 1];

                if (event.key === 'Tab') {
                    if (event.shiftKey) {
                        if (document.activeElement === firstElement) {
                            event.preventDefault();
                            lastElement.focus();
                        }
                    } else {
                        if (document.activeElement === lastElement) {
                            event.preventDefault();
                            firstElement.focus();
                        }
                    }
                }
            }
        });

        document.querySelectorAll('.color-box').forEach(box => {
            box.addEventListener('click', (event) => {
                event.preventDefault();
                const parentPair = box.parentElement;
                const fgName = parentPair.querySelector('.foreground-name').innerText;
                const fgHex = parentPair.querySelector('.foreground-name').nextSibling.textContent.trim();
                const bgName = parentPair.querySelector('.background-name').innerText;
                const bgHex = parentPair.querySelector('.background-name').nextSibling.textContent.trim();
                const contrastRatio = parentPair.querySelector('.contrast-info p:nth-child(3)').innerText.split(":")[1].trim();
                const levelSmall = parentPair.querySelector('.contrast-info p:nth-child(4)').innerText.split(":")[1].trim();
                const levelLarge = parentPair.querySelector('.contrast-info p:nth-child(5)').innerText.split(":")[1].trim();

                var html = '<p class="lang" data-lang="en"><strong>Foreground:</strong> ' + fgName + ' (' + fgHex + ')</p>' +
                           '<p class="lang" data-lang="jp" style="display:none;"><strong>Foreground:</strong> ' + fgName + ' (' + fgHex + ')</p>' +
                           '<p class="lang" data-lang="en"><strong>Background:</strong> ' + bgName + ' (' + bgHex + ')</p>' +
                           '<p class="lang" data-lang="jp" style="display:none;"><strong>Background:</strong> ' + bgName + ' (' + bgHex + ')</p>' +
                           '<p class="lang" data-lang="en"><strong>Contrast Ratio:</strong> ' + contrastRatio + '</p>' +
                           '<p class="lang" data-lang="jp" style="display:none;"><strong>コントラスト比:</strong> ' + contrastRatio + '</p>' +
                           '<p class="lang" data-lang="en"><strong>WCAG Level (Small Text):</strong> ' + levelSmall + '</p>' +
                           '<p class="lang" data-lang="jp" style="display:none;"><strong>WCAGレベル (小文字):</strong> ' + levelSmall + '</p>' +
                           '<p class="lang" data-lang="en"><strong>WCAG Level (Large Text):</strong> ' + levelLarge + '</p>' +
                           '<p class="lang" data-lang="jp" style="display:none;"><strong>WCAGレベル (大文字):</strong> ' + levelLarge + '</p>';

                const modalContent = document.querySelector('#modal-content');
                modalContent.innerHTML = '<button id="close-modal" aria-label="Close Modal">&times;</button>' + html;

                const newCloseModal = document.getElementById('close-modal');
                newCloseModal.addEventListener('click', () => {
                    modal.classList.remove('show');
                    if (showModalBtn) {
                        showModalBtn.focus();
                    }
                });

                modal.classList.add('show');
                newCloseModal.focus();
            });
        });

        const fgColorPicker = document.getElementById('foreground-color');
        const bgColorPicker = document.getElementById('background-color');
        const contrastRatioDisplay = document.getElementById('contrast-ratio');

        function hexToLuminance(hex) {
            hex = hex.replace('#', '');
            const r = parseInt(hex.substring(0, 2), 16) / 255;
            const g = parseInt(hex.substring(2, 4), 16) / 255;
            const b = parseInt(hex.substring(4, 6), 16) / 255;

            const linearize = (c) => {
                return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
            }

            const R = linearize(r);
            const G = linearize(g);
            const B = linearize(b);

            return 0.2126 * R + 0.7152 * G + 0.0722 * B;
        }

        function calculateContrast() {
            const fgHex = fgColorPicker.value;
            const bgHex = bgColorPicker.value;

            const fgLum = hexToLuminance(fgHex);
            const bgLum = hexToLuminance(bgHex);

            const L1 = Math.max(fgLum, bgLum);
            const L2 = Math.min(fgLum, bgLum);
            const ratio = (L1 + 0.05) / (L2 + 0.05);

            contrastRatioDisplay.textContent = ratio.toFixed(2);
        }

        fgColorPicker.addEventListener('input', calculateContrast);
        bgColorPicker.addEventListener('input', calculateContrast);

        calculateContrast();
    </script>
</body>
</html>
`
