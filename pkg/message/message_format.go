package message

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/fatih/color"
	"github.com/joho/godotenv"
)

type MyTime struct {
	CurrentTime time.Time
	Ack         bool
}

func MyError(err error, message string) {
	color.Red("******************** Error ********************")
	color.Red("Error happened in > %s", message)
	color.Red("Error Message > %s", err.Error())
	color.Red("***********************************************")
	os.Exit(1)
}

func MyWriteMessage(m any) {
	color.Green("[WRITE] %v", m)
}

func MyReadMessage(m any) {
	color.Green("[READ ] %v", m)
}

func MyMessage(s string) {
	color.Blue(s)
}

func MyExit(command string, processIds []int) {
	if command == "exit" {
		fmt.Println("Bye")
		// 1. 各プロセスの削除
		for _, pid := range processIds {
			process, err := os.FindProcess(pid)
			if err != nil {
				//fmt.Printf("%d is not found\n", pid)
				log.Fatal("FindProcess failed: ", err)
			}

			err = process.Signal(os.Interrupt)
			if err != nil {
				//fmt.Printf("%d is not interrupted\n", pid)
				log.Fatal("Interrupt failed: ", err)
			} else {
				fmt.Printf("process (%d) is killed\n", pid)
			}
		}

		// 2-0. パスを入手
		err := godotenv.Load(os.Getenv("HOME") + "/.env")
		if err != nil {
			log.Fatal(err)
		}

		// 2. GraphDB, SensingDBのレコード削除
		// GraphDB
		clear_graphdb_path := os.Getenv("HOME") + os.Getenv("PROJECT_PATH") + "/setup/GraphDB/clear_GraphDB.py"
		cmdGraphDB := exec.Command("python3", clear_graphdb_path)
		errCmdGraphDB := cmdGraphDB.Run()
		if errCmdGraphDB != nil {
			log.Fatal(errCmdGraphDB)
		}

		// SensingDB
		//hoge
		os.Exit(0)
	}
}
