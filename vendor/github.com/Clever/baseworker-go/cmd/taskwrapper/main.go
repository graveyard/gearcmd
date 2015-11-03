package main

import (
	"flag"
	"log"
	"os"

	baseworker "github.com/Clever/baseworker-go"
	"github.com/Clever/baseworker-go/taskwrapper"
)

func main() {
	functionName := flag.String("name", "", "Name of the Gearman function")
	functionCmd := flag.String("cmd", "", "The cmd to run")
	gearmanHost := flag.String("gearman-host", "localhost", "The Gearman host")
	gearmanPort := flag.String("gearman-port", "4730", "The Gearman port")
	flag.Parse()
	if len(*functionName) == 0 {
		log.Printf("Error: name not defined")
		flag.PrintDefaults()
		os.Exit(2)
	}
	if len(*functionCmd) == 0 {
		log.Printf("Error: cmd not defined")
		flag.PrintDefaults()
		os.Exit(3)
	}

	config := taskwrapper.TaskConfig{FunctionName: *functionName, FunctionCmd: *functionCmd, WarningLines: 5}
	worker := baseworker.NewWorker(*functionName, config.Process)
	defer worker.Close()
	log.Printf("Listening for job: " + *functionName)
	if err := worker.Listen(*gearmanHost, *gearmanPort); err != nil {
		log.Fatal(err)
	}
}
