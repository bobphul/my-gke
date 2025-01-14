package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"google.golang.org/api/container/v1"
	"google.golang.org/api/cloudresourcemanager/v1"
)

type GKEConfig struct {
	ProjectID  string
	Region     string
	Cluster    string
	Username   string
}

func getProjects(ctx context.Context) ([]string, error) {
	cloudResourceManagerService, err := cloudresourcemanager.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create cloud resource manager client: %v", err)
	}

	var projectIDs []string
	resp, err := cloudResourceManagerService.Projects.List().Do()
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %v", err)
	}

	for _, project := range resp.Projects {
		if project.LifecycleState == "ACTIVE" {
			projectIDs = append(projectIDs, project.ProjectId)
		}
	}

	return projectIDs, nil
}

func getClusters(ctx context.Context, projectID string) ([]*container.Cluster, error) {
	containerService, err := container.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create container service client: %v", err)
	}

	parent := fmt.Sprintf("projects/%s/locations/-", projectID)
	resp, err := containerService.Projects.Locations.Clusters.List(parent).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to list clusters: %v", err)
	}

	return resp.Clusters, nil
}

func getCurrentPublicIP() (string, error) {
	resp, err := http.Get("https://api.ipify.org")
	if err != nil {
		return "", fmt.Errorf("failed to get public IP: %v", err)
	}
	defer resp.Body.Close()

	ip, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %v", err)
	}

	return string(ip), nil
}

func getGcloudUsername() (string, error) {
	cmd := exec.Command("gcloud", "config", "get-value", "account")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get gcloud account: %v", err)
	}

	lines := strings.Split(string(output), "\n")
	var email string
	for _, line := range lines {
		if strings.Contains(line, "@") {
			email = strings.TrimSpace(line)
			break
		}
	}

	if email != "" {
		username := strings.Split(email, "@")[0]
		username = strings.ReplaceAll(username, ".", "-")
		return username, nil
	}

	return "", fmt.Errorf("no valid email found in gcloud config")
}

func updateAuthorizedNetworks(ctx context.Context, config GKEConfig, cluster *container.Cluster) error {
	containerService, err := container.NewService(ctx)
	if err != nil {
		return fmt.Errorf("failed to create container service client: %v", err)
	}

	publicIP, err := getCurrentPublicIP()
	if err != nil {
		return err
	}

	var currentNetworks []*container.CidrBlock
	if cluster.MasterAuthorizedNetworksConfig.CidrBlocks != nil {
		currentNetworks = cluster.MasterAuthorizedNetworksConfig.CidrBlocks
	}

	username := config.Username
	userNetworkExists := false
	for i, network := range currentNetworks {
		if network.DisplayName == username {
			currentNetworks[i].CidrBlock = publicIP + "/32"
			userNetworkExists = true
			break
		}
	}

	if !userNetworkExists {
		currentNetworks = append(currentNetworks, &container.CidrBlock{
			DisplayName: username,
			CidrBlock:   publicIP + "/32",
		})
	}

	req := &container.UpdateClusterRequest{
		Update: &container.ClusterUpdate{
			DesiredMasterAuthorizedNetworksConfig: &container.MasterAuthorizedNetworksConfig{
				Enabled:                     true,
				CidrBlocks:                 currentNetworks,
				GcpPublicCidrsAccessEnabled: cluster.MasterAuthorizedNetworksConfig.GcpPublicCidrsAccessEnabled,
			},
		},
	}

	name := fmt.Sprintf("projects/%s/locations/%s/clusters/%s",
		config.ProjectID, config.Region, config.Cluster)

	op, err := containerService.Projects.Locations.Clusters.Update(name, req).Do()
	if err != nil {
		return fmt.Errorf("failed to update authorized networks: %v", err)
	}

	return waitForOperation(ctx, containerService, op, config)
}

func waitForOperation(ctx context.Context, svc *container.Service, op *container.Operation, config GKEConfig) error {
	opName := fmt.Sprintf("projects/%s/locations/%s/operations/%s",
		config.ProjectID, config.Region, op.Name)

	for {
		result, err := svc.Projects.Locations.Operations.Get(opName).Do()
		if err != nil {
			return fmt.Errorf("failed to get operation status: %v", err)
		}

		if result.Status == "DONE" {
			if result.Error != nil {
				return fmt.Errorf("operation failed: %v", result.Error.Message)
			}
			return nil
		}

		time.Sleep(2 * time.Second)
	}
}

func hasAuthorizedNetworks(cluster *container.Cluster) bool {
	return cluster.MasterAuthorizedNetworksConfig != nil && 
		cluster.MasterAuthorizedNetworksConfig.Enabled
}

func setClusterCredentials(ctx context.Context, config GKEConfig, cluster *container.Cluster) error {
	fmt.Print("\n")

	if hasAuthorizedNetworks(cluster) {
		fmt.Printf("📡 Updating authorized networks...\n")
		if err := updateAuthorizedNetworks(ctx, config, cluster); err != nil {
			return fmt.Errorf("failed to update authorized networks: %v", err)
		}
		fmt.Printf("✨ Successfully updated authorized networks with your IP\n\n")
	} else {
		fmt.Printf("ℹ️  Cluster does not have authorized networks enabled, skipping IP update\n\n")
	}

	fmt.Printf("🔑 Configuring cluster credentials...\n")
	cmd := exec.Command("gcloud", "container", "clusters", "get-credentials",
		config.Cluster,
		"--region", config.Region,
		"--project", config.ProjectID)
	
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	
	if err := cmd.Run(); err != nil {
		return err
	}

	fmt.Printf("✅ Testing cluster connection...\n")
	testCmd := exec.Command("kubectl", "config", "current-context")
	testCmd.Stdout = io.Discard
	return testCmd.Run()
}

type model struct {
	choices    []string
	cursor     int
	selected   string
	step       string
	projects   []string
	clusters   []*container.Cluster
	projectID  string
	loading    bool
	program    *tea.Program
}

func initialModel() model {
	return model{
		step: "project",
	}
}

func (m *model) Init() tea.Cmd {
	return nil
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case "enter":
			if m.step == "project" {
				m.projectID = m.projects[m.cursor]
				m.step = "cluster"
				m.cursor = 0
				clusters, err := getClusters(context.Background(), m.projectID)
				if err != nil {
					log.Fatalf("Error getting clusters: %v", err)
				}
				m.clusters = clusters
				var clusterNames []string
				for _, cluster := range clusters {
					clusterNames = append(clusterNames, cluster.Name)
				}
				m.choices = clusterNames
			} else if m.step == "cluster" {
				selectedCluster := m.clusters[m.cursor]
				username, err := getGcloudUsername()
				if err != nil {
					log.Printf("Error getting gcloud username: %v", err)
					return m, tea.Quit
				}

				config := GKEConfig{
					ProjectID: m.projectID,
					Region:    selectedCluster.Location,
					Cluster:   selectedCluster.Name,
					Username:  username,
				}

				m.loading = true
				m.step = "configuring"
				
				go func() {
					if err := setClusterCredentials(context.Background(), config, selectedCluster); err != nil {
						log.Printf("Error setting cluster credentials: %v", err)
						m.loading = false
						m.program.Send(errMsg{err})
						return
					}
					m.program.Send(successMsg{cluster: selectedCluster.Name})
				}()

				return m, nil
			}
		}
	case errMsg:
		return m, tea.Quit
	case successMsg:
		fmt.Printf("\n✨ Successfully configured credentials for cluster: %s\n", msg.cluster)
		fmt.Printf("🚀 You can now use kubectl to interact with the cluster\n")
		fmt.Printf("📝 Current context: %s\n\n", msg.cluster)
		return m, tea.Quit
	}
	return m, nil
}

func (m *model) View() string {
	if m.loading {
		return "\n🔄 Configuring cluster access...\n"
	}

	var s strings.Builder
	s.WriteString("Select using ↑/↓ arrows and enter to confirm\n\n")

	if m.step == "project" {
		s.WriteString("Choose a GCP project:\n\n")
	} else {
		s.WriteString("Choose a GKE cluster:\n\n")
	}

	for i, choice := range m.choices {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}
		s.WriteString(fmt.Sprintf("%s %s\n", cursor, choice))
	}

	s.WriteString("\n(press q to quit)\n")
	return s.String()
}

type errMsg struct{ err error }
type successMsg struct{ cluster string }

func main() {
	ctx := context.Background()

	projects, err := getProjects(ctx)
	if err != nil {
		log.Fatalf("Error getting projects: %v", err)
	}

	m := &model{
		step:     "project",
		projects: projects,
		choices:  projects,
	}

	p := tea.NewProgram(m)
	m.program = p

	if _, err := p.Run(); err != nil {
		log.Fatalf("Error running program: %v", err)
	}
} 