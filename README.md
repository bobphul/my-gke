# GKE Cluster Access Manager

GKE Cluster Access Manager is a CLI tool that simplifies access management for Google Kubernetes Engine (GKE) clusters.

## Key Features

- List and select GCP projects
- List and select GKE clusters from the chosen project
- Easy cluster selection through interactive UI
- Automatic kubeconfig configuration
- Automatic Authorized Networks update (if enabled)

## Prerequisites

1. Go 1.20 or higher
2. Google Cloud SDK (gcloud)
3. kubectl
4. GCP account with required permissions
   - Container Engine related permissions
   - Cloud Resource Manager related permissions

## Installation

1. Clone repository
```bash
git clone https://github.com/bobphul/my-gke.git
cd my-gke
```

2. Install packages
```bash
go mod tidy
```

3. Build
```bash
go build -o gke
```

4. Add executable to PATH
   
For local environment:
```bash
sudo mv gke /usr/local/bin/
```

For Google Cloud Shell:
```bash
# Create Go bin directory if it doesn't exist
mkdir -p ~/gopath/bin
# Move the executable
mv gke ~/gopath/bin/
# Add to PATH if not already added
echo 'export PATH=$PATH:~/gopath/bin' >> ~/.bashrc
source ~/.bashrc
```

## Usage
1. Configure gcloud authentication (SKIP if using Google Cloud Shell)
```bash
gcloud auth login
gcloud auth application-default login
```

2. Run the program
```bash
gke
```

3. Use arrow keys (↑/↓) to select projects and clusters, press Enter to confirm

## Feature Details

- **Project Selection**: Displays all accessible GCP projects in your account
- **Cluster Selection**: Shows all GKE clusters in the selected project
- **Automatic Authentication**: Automatically configures kubeconfig for the selected cluster
- **IP Auto-update**: Automatically adds/updates the current user's IP to Authorized Networks if enabled

## Required GCP Permissions

- `container.clusters.get`
- `container.clusters.list`
- `container.clusters.update`
- `container.operations.get`
- `resourcemanager.projects.get`
- `resourcemanager.projects.list`

## Troubleshooting

1. If permission errors occur:
   - Verify gcloud authentication is properly set up
   - Check if necessary IAM permissions are granted

2. If cluster connection errors occur:
   - Check Authorized Networks settings
   - Verify VPC firewall rules

## Limitations

- IP auto-update feature is skipped if Authorized Networks is not enabled on the GKE cluster
- Private clusters may require additional network configuration

## License

This project is licensed under the MIT License. See the LICENSE file for details.
