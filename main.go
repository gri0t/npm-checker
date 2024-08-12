package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"
	"bufio"
    "regexp"
    "strings"
	"flag"
	"context"
	"strconv"
	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"golang.org/x/time/rate"

)

type PackageJson struct {
	Dependencies map[string]string `json:"dependencies"`
}

type RateLimiter struct {
    limiter *rate.Limiter
    remaining int
    reset time.Time
}

func main() {
    var gitdorkerFile string
    var githubToken string

    flag.StringVar(&gitdorkerFile, "gitdorker", "", "Path to GitDorker results file")
    flag.StringVar(&githubToken, "token", "", "GitHub Personal Access Token")
    flag.Parse()

	rateLimiter := NewRateLimiter(29) // GitHub's default rate limit

    if gitdorkerFile != "" {
        if githubToken == "" {
            fmt.Println("GitHub token is required for processing GitDorker results")
            os.Exit(1)
        }
        processGitDorkerResults(gitdorkerFile, githubToken,rateLimiter)
        return
    }

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
		exists, err := checkPackageExists(name, rateLimiter)
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

func processGitDorkerResults(filePath, token string, rateLimiter *RateLimiter) {
    file, err := os.Open(filePath)
    if err != nil {
        fmt.Printf("Error opening GitDorker results file: %v\n", err)
        return
    }
    defer file.Close()

    scanner := bufio.NewScanner(file)
    urlRegex := regexp.MustCompile(`https://github\.com/search\?q=.*filename%3Apackage\.json.*`)

    for scanner.Scan() {
        line := scanner.Text()
        if matches := urlRegex.FindStringSubmatch(line); len(matches) > 0 {
            url := matches[0]
            fmt.Printf("Processing results from: %s\n", url)
            processSearchResults(url, token, rateLimiter)
        }
    }

    if err := scanner.Err(); err != nil {
        fmt.Printf("Error reading GitDorker results file: %v\n", err)
    }

	for scanner.Scan() {
        line := scanner.Text()
        if matches := urlRegex.FindStringSubmatch(line); len(matches) > 0 {
            url := matches[0]
            fmt.Printf("Processing results from: %s\n", url)
            processSearchResults(url, token, rateLimiter)
        }
    }
}

func processSearchResults(searchURL, token string, rateLimiter *RateLimiter) {
    // Convert search URL to API URL
    apiURL := strings.Replace(searchURL, "https://github.com/search", "https://api.github.com/search/code", 1)
    
    req, _ := http.NewRequest("GET", apiURL, nil)
    req.Header.Set("Authorization", "token "+token)
    req.Header.Set("Accept", "application/vnd.github.v3+json")

	rateLimiter.Wait()
    rateLimiter.CheckRateLimit()

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        fmt.Printf("Error fetching search results: %v\n", err)
        return
    }
    defer resp.Body.Close()

	rateLimiter.UpdateFromHeaders(resp.Header)

    body, _ := ioutil.ReadAll(resp.Body)
    var result map[string]interface{}
    json.Unmarshal(body, &result)

    items, ok := result["items"].([]interface{})
    if !ok {
        fmt.Println("No results found")
        return
    }

    for _, item := range items {
        itemMap := item.(map[string]interface{})
        rawURL := itemMap["html_url"].(string)
        rawURL = strings.Replace(rawURL, "github.com", "raw.githubusercontent.com", 1)
        rawURL = strings.Replace(rawURL, "/blob/", "/", 1)

        fmt.Printf("Checking package.json: %s\n", rawURL)
        checkDependenciesFromURL(rawURL, token, rateLimiter)
    }
}

func checkDependenciesFromURL(url, token string, rateLimiter *RateLimiter) {
    req, _ := http.NewRequest("GET", url, nil)
    req.Header.Set("Authorization", "token "+token)

	rateLimiter.Wait()
    rateLimiter.CheckRateLimit()
    
    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        fmt.Printf("Error fetching package.json: %v\n", err)
        return
    }
    defer resp.Body.Close()

	rateLimiter.UpdateFromHeaders(resp.Header)

    body, _ := ioutil.ReadAll(resp.Body)
    var packageJson PackageJson
    err = json.Unmarshal(body, &packageJson)
    if err != nil {
        fmt.Printf("Error parsing package.json: %v\n", err)
        return
    }

    for name, version := range packageJson.Dependencies {
        fmt.Printf("Checking %s (version %s): ", name, version)
        exists, err := checkPackageExists(name, rateLimiter)
        if err != nil {
            color.Red("Error: %v", err)
            continue
        }
        if exists {
            color.Green("✔ Exists")
        } else {
            color.Red("✘ Does not exist")
        }
    }
    fmt.Println()
}

func NewRateLimiter(requestsPerMinute int) *RateLimiter {
    return &RateLimiter{
        limiter: rate.NewLimiter(rate.Limit(requestsPerMinute)/60, 1),
        remaining: requestsPerMinute,
        reset: time.Now().Add(time.Minute),
    }
}

func (rl *RateLimiter) Wait() {
    rl.limiter.Wait(context.Background())
}

func (rl *RateLimiter) UpdateFromHeaders(headers http.Header) {
    if remaining := headers.Get("X-RateLimit-Remaining"); remaining != "" {
        rl.remaining, _ = strconv.Atoi(remaining)
    }
    if reset := headers.Get("X-RateLimit-Reset"); reset != "" {
        resetTime, _ := strconv.ParseInt(reset, 10, 64)
        rl.reset = time.Unix(resetTime, 0)
    }
}

func (rl *RateLimiter) CheckRateLimit() {
    if rl.remaining <= 0 {
        waitTime := time.Until(rl.reset)
        if waitTime > 0 {
            color.Yellow(fmt.Sprintf("Rate limit reached. Waiting for %v before next request.", waitTime))
            time.Sleep(waitTime)
        }
    }
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

func checkPackageExists(name string, rateLimiter *RateLimiter) (bool, error) {
	url := fmt.Sprintf("https://registry.npmjs.org/%s", name)

	rateLimiter.Wait()
    rateLimiter.CheckRateLimit()

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
