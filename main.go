package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
)

type ContrastResult struct {
	Fg            string  `json:"fg"`
	Bg            string  `json:"bg"`
	ContrastRatio float64 `json:"contrastRatio"`
	LevelSmall    string  `json:"levelSmall"`
	LevelLarge    string  `json:"levelLarge"`
}

var lightColors = map[string]string{
	"black":   "#333333",
	"silver":  "#777777",
	"gray":    "#555555",
	"white":   "#eeeeee",
	"maroon":  "#800000",
	"red":     "#b30000",
	"purple":  "#800080",
	"fuchsia": "#b300b3",
	"green":   "#008000",
	"lime":    "#2d7f2d",
	"olive":   "#666600",
	"yellow":  "#999900",
	"navy":    "#000080",
	"blue":    "#0000b3",
	"teal":    "#006666",
	"aqua":    "#009999",
}

var darkColors = map[string]string{
	"black":   "#121212",
	"silver":  "#c0c0c0",
	"gray":    "#aaaaaa",
	"white":   "#f0f0f0",
	"maroon":  "#ff4d4d",
	"red":     "#ff3333",
	"purple":  "#cc66cc",
	"fuchsia": "#ff66ff",
	"green":   "#33cc33",
	"lime":    "#66ff66",
	"olive":   "#cccc33",
	"yellow":  "#ffff33",
	"navy":    "#6666ff",
	"blue":    "#4d4dff",
	"teal":    "#33cccc",
	"aqua":    "#66ffff",
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
	rVal, err := strconv.ParseInt(hex[0:2], 16, 64)
	if err != nil {
		return 0, err
	}
	gVal, err := strconv.ParseInt(hex[2:4], 16, 64)
	if err != nil {
		return 0, err
	}
	bVal, err := strconv.ParseInt(hex[4:6], 16, 64)
	if err != nil {
		return 0, err
	}

	R := toLinear(float64(rVal) / 255.0)
	G := toLinear(float64(gVal) / 255.0)
	B := toLinear(float64(bVal) / 255.0)

	return 0.2126*R + 0.7152*G + 0.0722*B, nil
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
	L1 := fgLum
	L2 := bgLum
	if L2 > L1 {
		L1, L2 = L2, L1
	}
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
	results := []ContrastResult{}

	for _, fgHex := range lightColors {
		for _, bgHex := range darkColors {
			ratio, err := contrastRatio(fgHex, bgHex)
			if err != nil {
				continue
			}
			levelSmall := complianceLevel(ratio)
			levelLarge := complianceLevelLarge(ratio)
			res := ContrastResult{
				Fg:            fgHex,
				Bg:            bgHex,
				ContrastRatio: math.Round(ratio*100) / 100,
				LevelSmall:    levelSmall,
				LevelLarge:    levelLarge,
			}
			results = append(results, res)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func main() {
	http.HandleFunc("/all-contrasts", allContrastsHandler)
	fmt.Println("サーバーがポート8080で起動しています...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
