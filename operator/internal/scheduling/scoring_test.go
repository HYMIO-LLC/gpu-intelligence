package scheduling

import (
	"math"
	"testing"
)

func TestScore(t *testing.T) {
	tests := []struct {
		name    string
		temp    float64
		util    float64
		maxTemp float64
		want    float64
	}{
		// 0.7 × idle + 0.3 × headroom, maxTemp = 80 unless noted.
		{name: "idle and cold", temp: 0, util: 0, maxTemp: 80, want: 100},
		{name: "typical", temp: 60, util: 50, maxTemp: 80, want: 0.7*50 + 0.3*25},
		{name: "lightly loaded and cool", temp: 40, util: 20, maxTemp: 80, want: 0.7*80 + 0.3*50},
		{name: "at temperature limit", temp: 80, util: 50, maxTemp: 80, want: 0.7 * 50},
		{name: "fully loaded at limit", temp: 80, util: 100, maxTemp: 80, want: 0},
		{name: "below zero celsius clamps headroom", temp: -10, util: 0, maxTemp: 80, want: 100},
		{name: "above limit clamps headroom", temp: 90, util: 0, maxTemp: 80, want: 70},
		{name: "utilization above 100 clamps idle", temp: 0, util: 150, maxTemp: 80, want: 30},
		{name: "negative utilization clamps idle", temp: 80, util: -5, maxTemp: 80, want: 70},
		{name: "NaN temperature", temp: math.NaN(), util: 0, maxTemp: 80, want: 0},
		{name: "Inf utilization", temp: 0, util: math.Inf(-1), maxTemp: 80, want: 0},
		{name: "zero max temperature", temp: 0, util: 0, maxTemp: 0, want: 0},
		{name: "negative max temperature", temp: 0, util: 0, maxTemp: -1, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Score(tt.temp, tt.util, tt.maxTemp)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("Score(%v, %v, %v) = %v, want %v",
					tt.temp, tt.util, tt.maxTemp, got, tt.want)
			}
			if got < 0 || got > 100 || !isFinite(got) {
				t.Fatalf("Score() = %v, want finite value in [0, 100]", got)
			}
		})
	}
}

func TestScoreMonotonic(t *testing.T) {
	const maxTemp = 80

	for temp := 0.0; temp <= maxTemp; temp += 5 {
		for util := 0.0; util < 100; util += 5 {
			base := Score(temp, util, maxTemp)
			if busier := Score(temp, util+5, maxTemp); busier >= base {
				t.Fatalf("Score(%v, %v) = %v not below Score(%v, %v) = %v",
					temp, util+5, busier, temp, util, base)
			}
			if temp+5 <= maxTemp {
				if hotter := Score(temp+5, util, maxTemp); hotter >= base {
					t.Fatalf("Score(%v, %v) = %v not below Score(%v, %v) = %v",
						temp+5, util, hotter, temp, util, base)
				}
			}
		}
	}
}
