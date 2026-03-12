package captcha

import (
	"math"
	"testing"
)

func TestAngleToPixels(t *testing.T) {
	tests := []struct {
		name       string
		angle      float32
		trackWidth float64
		knobWidth  float64
		want       float64
	}{
		{
			name:       "0 graus - 0 pixels",
			angle:      0,
			trackWidth: 340,
			knobWidth:  64,
			want:       0,
		},
		{
			name:       "180 graus - full track",
			angle:      180,
			trackWidth: 340,
			knobWidth:  64,
			want:       (340 - 64),
		},
		{
			name:       "90 graus - metade do track",
			angle:      90,
			trackWidth: 340,
			knobWidth:  64,
			want:       (340 - 64) * 0.5,
		},
		{
			name:       "360 graus - wrap para 0",
			angle:      360,
			trackWidth: 340,
			knobWidth:  64,
			want:       0,
		},
		{
			name:       "negativo -90 → -90+180 = 90",
			angle:      -90,
			trackWidth: 340,
			knobWidth:  64,
			want:       (340 - 64) * (90.0 / 180.0),
		},
		{
			name:       "negativo -45 → -45+180 = 135",
			angle:      -45,
			trackWidth: 348,
			knobWidth:  64,
			want:       (348 - 64) * (135.0 / 180.0),
		},
		{
			name:       "negativo -85 → -85+180 = 95",
			angle:      -85,
			trackWidth: 348,
			knobWidth:  64,
			want:       (348 - 64) * (95.0 / 180.0),
		},
		{
			name:       "120 graus - 2/3 do track",
			angle:      120,
			trackWidth: 348,
			knobWidth:  64,
			want:       (348 - 64) * (120.0 / 180.0),
		},
		{
			name:       "270 graus rebate para 90",
			angle:      270,
			trackWidth: 340,
			knobWidth:  64,
			want:       (340 - 64) * (90.0 / 180.0),
		},
		{
			name:       "track menor que knob retorna 0",
			angle:      90,
			trackWidth: 50,
			knobWidth:  64,
			want:       0,
		},
		{
			name:       "track igual knob retorna 0",
			angle:      90,
			trackWidth: 64,
			knobWidth:  64,
			want:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AngleToPixels(tt.angle, tt.trackWidth, tt.knobWidth)
			if math.Abs(got-tt.want) > 0.01 {
				t.Errorf("AngleToPixels(%v, %v, %v) = %v, want %v",
					tt.angle, tt.trackWidth, tt.knobWidth, got, tt.want)
			}
		})
	}
}
