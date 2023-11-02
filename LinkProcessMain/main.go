package main

import (
	"fmt"
	"mecm2m-Emulator/pkg/message"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/shirou/gopsutil/cpu"
)

func main() {
	loadEnv()

	var processIds int
	m2m_api_path := os.Getenv("HOME") + os.Getenv("PROJECT_NAME") + "/LinkProcess/main"
	cmdM2MAPI := exec.Command(m2m_api_path)
	errCmdM2MAPI := cmdM2MAPI.Start()
	if errCmdM2MAPI != nil {
		message.MyError(errCmdM2MAPI, "exec.Command > LinkProcess > Start")
	} else {
		fmt.Println("LinkProcess is running")
	}
	processIds = cmdM2MAPI.Process.Pid

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGALRM, syscall.SIGTERM)
	go func() {
		<-quit
		fmt.Println("ctrl-c pressed!")
		close(quit)
		process, err := os.FindProcess(processIds)
		if err != nil {
			message.MyError(err, "commandExecutionAfterEmulator > exit > os.FindProcess")
		}

		err = process.Signal(os.Interrupt)
		if err != nil {
			message.MyError(err, "commandExecutionAfterEmulator > exit > process.Signal")
		} else {
			fmt.Printf("process (%d) is killed\n", processIds)
		}
		os.Exit(0)
	}()

	threshold := 80.0

	for {
		cpuPercent, _ := cpu.Percent(time.Second, true)
		for i, usage := range cpuPercent {
			fmt.Printf("CPU%d Usage: %.2f%%\n", i, usage)

			if usage > threshold {
				fmt.Println("CPU Usage is larger than Threshold")
				process, err := os.FindProcess(processIds)
				if err != nil {
					message.MyError(err, "commandExecutionBeforeEmulator > start > os.FindProcess")
				}

				if err := process.Signal(syscall.Signal(syscall.SIGCONT)); err != nil {
					message.MyError(err, "commandExecutionBeforeEmulator > start > process.Signal")
				}
			}
		}
		time.Sleep(1 * time.Second)
	}

}

func loadEnv() {
	// .envファイルの読み込み
	if err := godotenv.Load(os.Getenv("HOME") + "/.env"); err != nil {
		message.MyError(err, "loadEnv > godotenv.Load")
	}
	mes := os.Getenv("SAMPLE_MESSAGE")
	// fmt.Printf("\x1b[32m%v\x1b[0m\n", message)
	message.MyMessage(mes)
}
