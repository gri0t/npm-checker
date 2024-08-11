package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"
	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
)

type PackageJson struct {
	Dependencies map[string]string `json:"dependencies"`
}

func main() {
	if len(os.Args) < 2 || os.Args[1] == "-h" || os.Args[1] == "-help" {
		displayHelp()
		return
	}

	packageJsonPath := os.Args[1]
	color.Cyan("Reading package.json from: %s", packageJsonPath)

	packageJson, err := readPackageJson(packageJsonPath)
	if err != nil {
		color.Red("Error reading package.json: %v", err)
		os.Exit(1)
	}

	color.Green("Successfully parsed package.json")
	fmt.Printf("Found %d dependencies\n\n", len(packageJson.Dependencies))

	existCount := 0
	notExistCount := 0

	for name, version := range packageJson.Dependencies {
		fmt.Printf("Checking %s (version %s): ", name, version)
		exists, err := checkPackageExists(name)
		if err != nil {
			color.Red("Error: %v", err)
			continue
		}
		if exists {
			color.Green("✔ Exists")
			existCount++
		} else {
			color.Red("✘ Does not exist")
			notExistCount++
		}
		time.Sleep(100 * time.Millisecond) // To prevent rate limiting
	}

	fmt.Printf("\nSummary:\n")
	color.Green("  ✔ %d packages exist", existCount)
	color.Red("  ✘ %d packages do not exist", notExistCount)
}

func readPackageJson(path string) (PackageJson, error) {
	var packageJson PackageJson

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return packageJson, err
	}

	err = json.Unmarshal(data, &packageJson)
	if err != nil {
		return packageJson, err
	}

	return packageJson, nil
}

func checkPackageExists(name string) (bool, error) {
	url := fmt.Sprintf("https://registry.npmjs.org/%s", name)
	resp, err := http.Get(url)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

func displayHelp() {
	logo := `
_   _ ____  __  __    ____ _   _ _____ ____ _  _______ ____  
| \ | |  _ \|  \/  |  / ___| | | | ____/ ___| |/ / ____|  _ \ 
|  \| | |_) | |\/| | | |   | |_| |  _|| |   | ' /|  _| | |_) |
| |\  |  __/| |  | | | |___| ___ | |__| |___| . \| |___|  _ < 
|_| \_|_|   |_|  |_|  \____|_| |_|_____\____|_|\_\_____|_| \_\
`
	fmt.Println(logo)
	color.Cyan("Developed by gri0t")
	fmt.Println()

	color.Yellow("Description:")
	fmt.Println("npm-checker is a tool to validate dependencies in a package.json file against the npm registry.")
	fmt.Println()

	color.Yellow("Usage:")
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Command", "Description"})
	table.SetBorder(false)
	table.Append([]string{"npm-checker /path/to/package.json", "Check dependencies in the specified package.json file"})
	table.Append([]string{"npm-checker -h, npm-checker -help", "Display this help message"})
	table.Render()
	fmt.Println()

	color.Yellow("Example:")
	fmt.Println("npm-checker /path/to/package.json")
	fmt.Println()
}
