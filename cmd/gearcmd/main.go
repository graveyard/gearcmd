package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/Clever/baseworker-go"
	"github.com/Clever/gearcmd/gearcmd"
)

func main() {
	functionName := flag.String("name", "", "Name of the Gearman function")
	functionCmd := flag.String("cmd", "", "The command to run")
	gearmanHost := flag.String("host", "", "The Gearman host. If not specified the GEARMAN_HOST environment variable will be used")
	gearmanPort := flag.String("port", "", "The Gearman port. If not specified the GEARMAN_PORT environment variable will be used")
	parseArgs := flag.Bool("parseargs", true, "If false send the job payload directly to the cmd as its first argument without parsing it")
	printVersion := flag.Bool("version", false, "Print the version and exit")
	cmdTimeout := flag.Duration("cmdtimeout", 0, "Maximum time for the command to run before it will be killed, e.g. 2h, 30m, 2h30m")
	flag.Parse()

	if *printVersion {
		fmt.Println(Version)
		os.Exit(0)
	}

	if len(*gearmanHost) == 0 {
		hostEnv := os.Getenv("GEARMAN_HOST")
		if len(hostEnv) == 0 {
			exitWithError("must either specify a host argument or set the GEARMAN_HOST environment variable")
		}
		*gearmanHost = hostEnv
	}

	if len(*gearmanPort) == 0 {
		portEnv := os.Getenv("GEARMAN_PORT")
		if len(portEnv) == 0 {
			exitWithError("must either specify a port argument or set the GEARMAN_PORT environment variable")
		}
		*gearmanPort = portEnv
	}

	if len(*functionName) == 0 {
		exitWithError("name not defined")
	}
	if len(*functionCmd) == 0 {
		exitWithError("cmd not defined")
	}

	config := gearcmd.TaskConfig{
		FunctionName: *functionName,
		FunctionCmd:  *functionCmd,
		WarningLines: 5,
		ParseArgs:    *parseArgs,
		CmdTimeout:   *cmdTimeout,
	}
	worker := baseworker.NewWorker(*functionName, config.Process)
	defer worker.Close()
	log.Printf("Listening for job: " + *functionName)
	if err := worker.Listen(*gearmanHost, *gearmanPort); err != nil {
		log.Fatal(err)
	}
}

// exitWithError prints out an error message and exits the process with an exit code of 1
func exitWithError(errorStr string) {
	log.Printf("Error: %s", errorStr)
	flag.PrintDefaults()
	os.Exit(1)

}
