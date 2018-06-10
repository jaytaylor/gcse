package gcse

import (
	"fmt"
	"strings"
)

type Person struct {
	Id       string
	Packages []string
}

func SitePersonID(site, username string) string {
	return fmt.Sprintf("%s:%s", site, username)
}

func ParsePersonID(id string) (site, username string) {
	parts := strings.Split(id, ":")
	return parts[0], parts[1]
}
