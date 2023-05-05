package m2mapp

import "fmt"

type App struct {
	AppID       string
	Address     string
	Description string
	GID         uint64 // goroutine ID
}

func (a *App) String() string {
	return fmt.Sprintf("AppID: %s, Address: %s, Description: %s, GID: %d", a.AppID, a.Address, a.Description, a.GID)
}
