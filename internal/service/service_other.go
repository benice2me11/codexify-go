//go:build !windows && !darwin && !linux

package service

import (
	"errors"

	"github.com/benice2me11/codexify-go/internal/config"
)

type Status struct {
	Installed bool
	State     int
}

func IsRunning(Status) bool { return false }

func StateString(int) string { return "unsupported" }

func unsupported() error {
	return errors.New("native service management is currently implemented only on Windows")
}

func Install(string, string, config.Config) error { return unsupported() }
func Start(string) error                          { return unsupported() }
func Stop(string) error                           { return unsupported() }
func Restart(string) error                        { return unsupported() }
func Remove(string) error                         { return unsupported() }
func Query(string) (Status, error)                { return Status{}, unsupported() }
func Run(string, config.Config, string) error     { return unsupported() }
func ExecutablePath() (string, error)             { return "", unsupported() }
