package envar

import (
	"os"
	"path/filepath"
)

const (
	OMYKUBE_HOME = "OMYKUBE_HOME"
)

func UserHome() string {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	return home
}

func OhMyKubeHome() string {
	home := os.Getenv(OMYKUBE_HOME)
	if home == "" {
		return filepath.Join(UserHome(), ".ohmykube")
	}
	return home
}

func OhMyKubeCacheDir() string {
	return filepath.Join(OhMyKubeHome(), "cache")
}

func OhMyKubeConfigDir() string {
	return filepath.Join(OhMyKubeHome(), "config")
}

func OhMyKubeLogDir() string {
	return filepath.Join(OhMyKubeHome(), "log")
}

func IsEnableDefaultMiror() bool {
	return os.Getenv("OHMYKUBE_ENABLE_MIRROR") == "1"
}

func OhMyKubeLimaHome() string {
	return filepath.Join(OhMyKubeHome(), ".lima")
}
