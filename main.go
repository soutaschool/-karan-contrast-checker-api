package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// ColorSets represents the structure of the input JSON file
type ColorSets struct {
	Light map[string]string `json:"light"`
	Dark  map[string]string `json:"dark"`
}

// ContrastResult holds the contrast ratio and WCAG levels for a color pair
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

// WCAGLevels categorizes the contrast ratio
type WCAGLevels struct {
	AAA []ContrastResult `json:"AAA"`
	AA  []ContrastResult `json:"AA"`
	Fail []ContrastResult `json:"Fail"`
}

// LoadColors reads and parses the colors from a JSON file
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

// toLinear converts sRGB values to linear RGB
func toLinear(value float64) float64 {
	if value <= 0.03928 {
		return value / 12.92
	}
	return math.Pow((value+0.055)/1.055, 2.4)
}

// relativeLuminance calculates the relative luminance of a color
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

// parseHex converts a 2-character hex string to an integer
func parseHex(h string) (int64, error) {
	return strconv.ParseInt(h, 16, 64)
}

// contrastRatio calculates the contrast ratio between two colors
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

// complianceLevel determines the WCAG level for small text
func complianceLevel(ratio float64) string {
	if ratio >= 7 {
		return "AAA"
	} else if ratio >= 4.5 {
		return "AA"
	} else {
		return "Fail"
	}
}

// complianceLevelLarge determines the WCAG level for large text
func complianceLevelLarge(ratio float64) string {
	if ratio >= 4.5 {
		return "AAA"
	} else if ratio >= 3 {
		return "AA"
	} else {
		return "Fail"
	}
}

// allContrastsHandler handles the root endpoint and renders the HTML page
func allContrastsHandler(w http.ResponseWriter, r *http.Request) {
	// Load colors from JSON file
	colors, err := LoadColors("colors.json")
	if err != nil {
		http.Error(w, "Failed to load colors: "+err.Error(), http.StatusInternalServerError)
		return
	}

	results := WCAGLevels{
		AAA: []ContrastResult{},
		AA:  []ContrastResult{},
		Fail: []ContrastResult{},
	}

	// Iterate through all light and dark color combinations
	for nameLight, fgHex := range colors.Light {
		for nameDark, bgHex := range colors.Dark {
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
				ContrastRatio:  math.Round(ratio*100) / 100, // Round to 2 decimal places
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
				results.Fail = append(results.Fail, result)
			}
		}
	}

	// Parse and execute the HTML template
	tmpl, err := template.New("results").Parse(htmlTemplate)
	if err != nil {
		http.Error(w, "Failed to parse template: "+err.Error(), http.StatusInternalServerError)
		return
	}

	data := struct {
		AAA  []ContrastResult
		AA   []ContrastResult
		Fail []ContrastResult
	}{
		AAA:  results.AAA,
		AA:   results.AA,
		Fail: results.Fail,
	}

	w.Header().Set("Content-Type", "text/html")
	err = tmpl.Execute(w, data)
	if err != nil {
		http.Error(w, "Failed to execute template: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

const htmlTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Contrast Checker Results</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            background-color: #f4f4f4;
            color: #333;
            margin: 0;
            padding: 20px;
        }
        h1, h2 {
            color: #444;
        }
        .container {
            max-width: 1200px;
            margin: auto;
        }
        .category {
            margin-bottom: 40px;
        }
        .color-pair {
            display: flex;
            align-items: center;
            background-color: #fff;
            padding: 15px;
            margin-bottom: 15px;
            border-radius: 8px;
            box-shadow: 0 2px 5px rgba(0,0,0,0.1);
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
        }
        .contrast-info {
            flex-grow: 1;
        }
        .fail {
            border-left: 5px solid #e74c3c;
            background-color: #fdecea;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>Contrast Checker Results</h1>

        {{if .AAA}}
        <div class="category">
            <h2>AAA Compliance</h2>
            {{range .AAA}}
            <div class="color-pair">
                <div class="color-box" style="background-color: {{.ForegroundHex}};">
                    FG
                </div>
                <div class="color-box" style="background-color: {{.BackgroundHex}};">
                    BG
                </div>
                <div class="contrast-info">
                    <p><strong>Foreground:</strong> {{.ForegroundName}} ({{.ForegroundHex}})</p>
                    <p><strong>Background:</strong> {{.BackgroundName}} ({{.BackgroundHex}})</p>
                    <p><strong>Contrast Ratio:</strong> {{.ContrastRatio}}</p>
                    <p><strong>WCAG Level (Small Text):</strong> {{.LevelSmallText}}</p>
                    <p><strong>WCAG Level (Large Text):</strong> {{.LevelLargeText}}</p>
                </div>
            </div>
            {{end}}
        </div>
        {{end}}

        {{if .AA}}
        <div class="category">
            <h2>AA Compliance</h2>
            {{range .AA}}
            <div class="color-pair">
                <div class="color-box" style="background-color: {{.ForegroundHex}};">
                    FG
                </div>
                <div class="color-box" style="background-color: {{.BackgroundHex}};">
                    BG
                </div>
                <div class="contrast-info">
                    <p><strong>Foreground:</strong> {{.ForegroundName}} ({{.ForegroundHex}})</p>
                    <p><strong>Background:</strong> {{.BackgroundName}} ({{.BackgroundHex}})</p>
                    <p><strong>Contrast Ratio:</strong> {{.ContrastRatio}}</p>
                    <p><strong>WCAG Level (Small Text):</strong> {{.LevelSmallText}}</p>
                    <p><strong>WCAG Level (Large Text):</strong> {{.LevelLargeText}}</p>
                </div>
            </div>
            {{end}}
        </div>
        {{end}}

        {{if .Fail}}
        <div class="category">
            <h2>Fail Compliance</h2>
            {{range .Fail}}
            <div class="color-pair fail">
                <div class="color-box" style="background-color: {{.ForegroundHex}};">
                    FG
                </div>
                <div class="color-box" style="background-color: {{.BackgroundHex}};">
                    BG
                </div>
                <div class="contrast-info">
                    <p><strong>Foreground:</strong> {{.ForegroundName}} ({{.ForegroundHex}})</p>
                    <p><strong>Background:</strong> {{.BackgroundName}} ({{.BackgroundHex}})</p>
                    <p><strong>Contrast Ratio:</strong> {{.ContrastRatio}}</p>
                    <p><strong>WCAG Level (Small Text):</strong> {{.LevelSmallText}}</p>
                    <p><strong>WCAG Level (Large Text):</strong> {{.LevelLargeText}}</p>
                    <p><strong>Action Required:</strong> Fix the color combination.</p>
                </div>
            </div>
            {{end}}
        </div>
        {{end}}
    </div>
</body>
</html>
`

func main() {
	// Check if colors.json exists
	if _, err := ioutil.ReadFile("colors.json"); err != nil {
		log.Fatalf("colors.json file not found. Please ensure it exists in the current directory.")
	}

	// Handle root path
	http.HandleFunc("/", allContrastsHandler)

	// Start the server in a separate goroutine
	go func() {
		fmt.Println("Server is running on http://localhost:8080/")
		if err := http.ListenAndServe(":8080", nil); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Automatically open the browser
	openBrowser("http://localhost:8080/")

	// Block main goroutine
	select {}
}

// openBrowser attempts to open the default browser to the specified URL
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
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
		args = []string{url}
	}

	err := exec.Command(cmd, args...).Start()
	if err != nil {
		log.Printf("Failed to open browser: %v", err)
	}
}
