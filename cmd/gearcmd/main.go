package main

import (
	"flag"
	"log"
	"os"

	"github.com/Clever/gearcmd/gearcmd"
)

func main() {
	functionName := flag.String("name", "", "Name of the Gearman function")
	functionCmd := flag.String("cmd", "", "The command to run")
	gearmanHost := flag.String("host", "localhost", "The Gearman host")
	gearmanPort := flag.String("port", "4730", "The Gearman port")
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

	config := gearcmd.TaskConfig{FunctionName: *functionName, FunctionCmd: *functionCmd, WarningLines: 5}
	worker := baseworker.NewWorker(*functionName, config.Process)
	defer worker.Close()
	log.Printf("Listening for job: " + *functionName)
	if err := worker.Listen(*gearmanHost, *gearmanPort); err != nil {
		log.Fatal(err)
	}
}
