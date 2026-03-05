package utils

import "testing"

func TestNormalizeAndValidateIPv4Range(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:    "compact format",
			input:   "192.168.64.200-192.168.64.210",
			want:    "192.168.64.200 - 192.168.64.210",
			wantErr: false,
		},
		{
			name:    "spaced format",
			input:   "192.168.64.200 - 192.168.64.210",
			want:    "192.168.64.200 - 192.168.64.210",
			wantErr: false,
		},
		{
			name:    "invalid start ip",
			input:   "999.999.999.999-192.168.64.210",
			wantErr: true,
		},
		{
			name:    "different subnet",
			input:   "192.168.64.200-192.168.65.210",
			wantErr: true,
		},
		{
			name:    "start greater than end",
			input:   "192.168.64.210-192.168.64.200",
			wantErr: true,
		},
		{
			name:    "ipv6 unsupported",
			input:   "::1-::2",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeAndValidateIPv4Range(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.want != "" && got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
