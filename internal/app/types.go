package app

import (
	"fmt"
	"log/slog"
	"os"
)

// ShootCoordinate represents the coordinates of a shoot cluster. It can be used to represent both the shoot and seed.
type ShootCoordinate struct {
	Project string
	Name    string
}

func (sc ShootCoordinate) GetNamespace() string {
	if sc.Project == "garden" {
		return "garden"
	}
	return fmt.Sprintf("garden-%s", sc.Project)
}

// Config is the application configuration.
type Config struct {
	Garden           string
	ReferenceShoot   ShootCoordinate
	TargetShoot      *ShootCoordinate
	BinaryAssetsPath string
}

type Exit struct {
	Err  error
	Code int
}

func ExitApp(code int) {
	panic(Exit{Code: code})
}

func ExitAppWithError(code int, err error) {
	panic(Exit{Code: code, Err: err})
}

func OnExit() {
	if r := recover(); r != nil {
		if exit, ok := r.(Exit); ok {
			if exit.Err != nil {
				slog.Error("Exiting with error", "error", exit.Err)
			}
			os.Exit(exit.Code)
		}
		panic(r)
	}
}
