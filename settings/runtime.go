package settings

import (
	"log"
	"os"
	"runtime"
	"strconv"
)

// ConfigureRuntime configures the server runtime, including the number of CPUs to use.
func ConfigureRuntime() {
	// Note that we're using os instead of syscall because we'll be parsing the int anyway, so there is no need to check if the envvar was found.
	useNumCPUsStr := os.Getenv("MAX_CPUS")
	useNumCPUsInt, err := strconv.ParseInt(useNumCPUsStr, 10, 0)
	useNumCPUs := int(useNumCPUsInt)
	if err != nil {
		useNumCPUs = runtime.NumCPU()
	}
	runtime.GOMAXPROCS(useNumCPUs)
	log.Printf("Running with %d CPUs.\n", useNumCPUs)
}
