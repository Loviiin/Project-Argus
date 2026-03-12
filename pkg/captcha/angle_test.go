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
			name:       "360 graus - wrap para 0",
			angle:      360,
			trackWidth: 340,
			knobWidth:  64,
			want:       0,
		},
		{
			name:       "180 graus - metade",
			angle:      180,
			trackWidth: 340,
			knobWidth:  64,
			want:       (340 - 64) * 0.5,
		},
		{
			name:       "90 graus - um quarto",
			angle:      90,
			trackWidth: 340,
			knobWidth:  64,
			want:       (340 - 64) * 0.25,
		},
		{
			name:       "negativo -90 normaliza para 270",
			angle:      -90,
			trackWidth: 340,
			knobWidth:  64,
			want:       (340 - 64) * (270.0 / 360.0),
		},
		{
			name:       "negativo -180 normaliza para 180",
			angle:      -180,
			trackWidth: 340,
			knobWidth:  64,
			want:       (340 - 64) * 0.5,
		},
		{
			name:       "45 graus dimensoes variaveis",
			angle:      45,
			trackWidth: 400,
			knobWidth:  50,
			want:       (400 - 50) * (45.0 / 360.0),
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
