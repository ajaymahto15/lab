package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
)

// Function to execute shell commands and handle errors
func execCmd(cmd *exec.Cmd) (string, error) {
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("command '%s' failed: %w\nOutput: %s", cmd.String(), err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

// Function to generate the docker-compose.yml file content
func generateComposeContent(numMachines int, image string) string {
	composeContent := fmt.Sprintf(`version: '3.8'
services:
  bastion:
    container_name: bastion
    hostname: bastion
    image: %s
    ports:
      - "220:22"
    networks:
      - bridge
`, image)

	for i := 1; i <= numMachines; i++ {
		hostPort := 220 + i
		serviceName := fmt.Sprintf("minion-%d", i)
		composeContent += fmt.Sprintf(`
  %s:
    container_name: %s
    hostname: %s
    image: %s
    ports:
      - "%d:22"
    networks:
      - bridge
`, serviceName, serviceName, serviceName, image, hostPort)
	}
	return composeContent
}

// Function to update /etc/hosts efficiently
func updateHosts(ip, hostname string) error {
	file, err := os.OpenFile("/etc/hosts", os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("opening /etc/hosts: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lines := []string{}
	for scanner.Scan() {
		line := scanner.Text()
		if !regexp.MustCompile(fmt.Sprintf(`^\s*%s\s*`, hostname)).MatchString(line) {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanning /etc/hosts: %w", err)
	}

	newEntry := fmt.Sprintf("%s %s\n", ip, hostname)
	newFileContent := strings.Join(append(lines, newEntry), "\n")
	_, err = file.Seek(0, 0)
	if err != nil {
		return fmt.Errorf("seeking /etc/hosts: %w", err)
	}
	_, err = file.WriteString(newFileContent)
	if err != nil {
		return fmt.Errorf("writing to /etc/hosts: %w", err)
	}
	return file.Truncate(int64(len(newFileContent)))
}

// Function to update /etc/hosts concurrently
func updateHostsConcurrent(ips []string, hostnames []string) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(ips))
	for i := range ips {
		wg.Add(1)
		go func(ip, hostname string) {
			defer wg.Done()
			if err := updateHosts(ip, hostname); err != nil {
				errChan <- fmt.Errorf("updating /etc/hosts for %s: %w", hostname, err)
			}
		}(ips[i], hostnames[i])
	}
	wg.Wait()
	close(errChan)
	for err := range errChan {
		return err //Return first error encountered
	}
	return nil
}

func cleanupContainers() error {
	cmd := exec.Command("docker", "compose", "down")
	_, err := execCmd(cmd)
	if err != nil {
		return fmt.Errorf("cleaning up containers: %w", err)
	}
	fmt.Println("Containers stopped and removed successfully.")
	return nil
}

func sshLoginBastion() error {
	cmd := exec.Command("ssh", "-p", "220", "minion@localhost", "-o", "StrictHostKeyChecking=no")
	cmd.Stdin = strings.NewReader("minion\n")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("ssh login to bastion failed: %w", err)
	}
	return nil
}

func main() {
	var numMachines int
	var cleanup bool
	var image string

	flag.IntVar(&numMachines, "machines", 0, "Number of minion services to create (must be > 0)")
	flag.BoolVar(&cleanup, "cleanup", false, "Cleanup containers after execution (default: false)")
	flag.StringVar(&image, "image", "schooleon/minion", "Docker image to use for containers")
	flag.Parse()

	if numMachines <= 0 {
		log.Fatal("Error: --machines flag must be a positive integer.")
	}

	composeContent := generateComposeContent(numMachines, image)
	if err := os.WriteFile("docker-compose.yml", []byte(composeContent), 0644); err != nil {
		log.Fatalf("Error writing docker-compose.yml: %v", err)
	}
	fmt.Println("docker-compose.yml generated successfully.")

	if _, err := execCmd(exec.Command("docker", "compose", "up", "-d")); err != nil {
		log.Fatalf("Error starting containers: %v", err)
	}
	fmt.Println("Containers started successfully.")

	cmd := exec.Command("docker", "compose", "ps", "-q")
	containerIDs, err := execCmd(cmd)
	if err != nil {
		log.Fatalf("Error getting container IDs: %v", err)
	}
	ids := strings.Fields(containerIDs)
	if len(ids) != numMachines+1 {
		log.Fatalf("Unexpected number of containers found. Expected %d, got %d", numMachines+1, len(ids))
	}

	ips := make([]string, len(ids))
	hostnames := make([]string, len(ids))
	var wg sync.WaitGroup
	errChan := make(chan error, len(ids))

	for i, id := range ids {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			inspectCmd := exec.Command("docker", "inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", id)
			ip, err := execCmd(inspectCmd)
			if err != nil {
				errChan <- fmt.Errorf("inspecting container %s: %w", id, err)
				return
			}
			hostname := "bastion"
			if i > 0 {
				hostname = fmt.Sprintf("minion-%d", i)
			}
			ips[i] = ip
			hostnames[i] = hostname
		}(i, id)
	}
	wg.Wait()
	close(errChan)
	for err := range errChan {
		log.Fatalf("Error during container inspection: %v", err)
	}

	if err := updateHostsConcurrent(ips, hostnames); err != nil {
		log.Fatalf("Error updating /etc/hosts: %v", err)
	}

	fmt.Println("\nAttempting SSH login to bastion (using password 'minion' - HIGHLY INSECURE!):")
	if err := sshLoginBastion(); err != nil {
		log.Printf("Bastion login failed: %v", err)
	} else {
		fmt.Println("Successfully logged into bastion.")
	}

	fmt.Println("\nSSH Connection Instructions (using your SSH key):")
	fmt.Println("ssh minion@bastion")
	for i := 1; i <= numMachines; i++ {
		fmt.Printf("ssh minion@minion-%d\n", i)
	}

	if cleanup {
		if err := cleanupContainers(); err != nil {
			log.Fatalf("Error cleaning up containers: %v", err)
		}
	}
}
