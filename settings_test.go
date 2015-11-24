package main

import (
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"os"
	"testing"
)

// TestSettings tests stuff from runtime.go
func TestSettings(t *testing.T) {
	Convey("The settings tests, ", t, func() {
		chks := []struct {
			envvar   string
			chkFunc  interface{}
			expPanic bool
		}{
			{"AWS_ACCESS_KEY_ID", CheckEnvVars, true},
			{"LOG_LEVEL", ConfigureLogger, false},
		}
		for i := range chks {
			chk := chks[i]
			Convey(fmt.Sprintf("Unsetting %s", chk.envvar), func() {
				curVal := os.Getenv(chk.envvar)
				os.Unsetenv(chk.envvar)
				if chk.expPanic {
					So(chk.chkFunc.(func()), ShouldPanic)
				} else {
					So(chk.chkFunc.(func()), ShouldNotPanic)
				}
				os.Setenv(chk.envvar, curVal)
			})
		}

		envvar := "LOG_LEVEL"
		Convey(fmt.Sprintf("Setting %s to an invalid level", envvar), func() {
			curVal := os.Getenv(envvar)
			os.Setenv(envvar, "CARROTS")
			So(ConfigureLogger, ShouldNotPanic)
			os.Setenv(envvar, curVal)
		})

		envfuncs := map[string]interface{}{"FETCH_OFFSET": FetchOffset, "FETCH_LIMIT": FetchLimit}
		for envvar, fun := range envfuncs {
			Convey(fmt.Sprintf("Setting %s to -2", envvar), func() {
				curVal := os.Getenv(envvar)
				os.Setenv(envvar, "-2")
				So(func() { fun.(func() int)() }, ShouldPanic)
				os.Setenv(envvar, curVal)
			})
		}
		
		Convey("intFromEnvVar returns the default value if envvar does not exist", func() {
			So(intFromEnvVar("SOME_VALUE_THAT_DOES_NOT_EXISTS", 19), ShouldEqual, 19)
		})
	})
}
