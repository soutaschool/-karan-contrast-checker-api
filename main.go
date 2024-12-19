package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
)

// ColorSets represents the structure of the input JSON file
type ColorSets struct {
	Light map[string]string `json:"light"`
	Dark  map[string]string `json:"dark"`
}

// ContrastResult holds the contrast ratio and WCAG levels for a color pair
type ContrastResult struct {
	Foreground     string  `json:"foreground"`
	Background     string  `json:"background"`
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
	return strconvParseInt(h, 16)
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

// allContrastsHandler handles the /all-contrasts endpoint
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
				Foreground:     fmt.Sprintf("%s (%s)", fgHex, nameLight),
				Background:     fmt.Sprintf("%s (%s)", bgHex, nameDark),
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// strconvParseInt is a helper function to parse hex strings
func strconvParseInt(s string, base int) (int64, error) {
	return strconv.ParseInt(s, base, 64)
}

func main() {
	// Check if colors.json exists
	if _, err := os.Stat("colors.json"); os.IsNotExist(err) {
		log.Fatal("colors.json file not found. Please ensure it exists in the current directory.")
	}

	r := mux.NewRouter()
	r.HandleFunc("/all-contrasts", allContrastsHandler).Methods("GET")

	fmt.Println("Server is running on port 8080...")
	log.Fatal(http.ListenAndServe(":8080", r))
}
