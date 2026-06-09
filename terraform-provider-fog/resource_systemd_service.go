package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"text/template"
	"os"
	"path/filepath"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Ensure implementation satisfies the expected interfaces.
var _ resource.Resource = (*systemdServiceResource)(nil)
var _ resource.ResourceWithImportState = (*systemdServiceResource)(nil)
var _ resource.ResourceWithModifyPlan = (*systemdServiceResource)(nil)

type systemdServiceResource struct {
	provider *fogProvider
}

type systemdServiceResourceModel struct {
	ID               types.String `tfsdk:"id"`
	NodeAddress      types.String `tfsdk:"node_address"`
	NodeUser         types.String `tfsdk:"node_user"`
	ServiceName      types.String `tfsdk:"service_name"`
	ExecStart        types.String `tfsdk:"exec_start"`
	Environment      types.Map    `tfsdk:"environment"`
	WorkingDirectory types.String `tfsdk:"working_directory"`
	UnitFileHash     types.String `tfsdk:"unit_file_hash"`
}

func (r *systemdServiceResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_systemd_service"
}

func (r *systemdServiceResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a systemd service on a remote fog node via SSH.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Internal ID of the resource.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"node_address": schema.StringAttribute{
				Description: "IP address or hostname of the fog node.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"node_user": schema.StringAttribute{
				Description: "SSH user override for this node. Falls back to provider config.",
				Optional:    true,
			},
			"service_name": schema.StringAttribute{
				Description: "Name of the systemd service (e.g., mqtt_publisher).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"exec_start": schema.StringAttribute{
				Description: "Command to execute.",
				Required:    true,
			},
			"environment": schema.MapAttribute{
				Description: "Environment variables for the service.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"working_directory": schema.StringAttribute{
				Description: "Working directory for the service.",
				Optional:    true,
			},
			"unit_file_hash": schema.StringAttribute{
				Description: "SHA256 hash of the rendered unit file. Used for drift detection.",
				Computed:    true,
			},
		},
	}
}

func (r *systemdServiceResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	provider, ok := req.ProviderData.(*fogProvider)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Provider Data Type",
			fmt.Sprintf("Expected *fogProvider, got: %T", req.ProviderData),
		)
		return
	}
	r.provider = provider
}

const unitTemplate = `[Unit]
Description={{.ServiceName}}

[Service]
Type=simple
ExecStart={{.ExecStart}}
{{- if .WorkingDirectory }}
WorkingDirectory={{.WorkingDirectory}}
{{- end }}
{{- range .Environment }}
Environment={{.}}
{{- end }}
Restart=on-failure

[Install]
WantedBy=multi-user.target
`

type unitTemplateData struct {
	ServiceName      string
	ExecStart        string
	WorkingDirectory string
	Environment      []string // Changed to string slice for deterministic sorting
}

func renderUnitFile(data unitTemplateData) (content string, hash string, err error) {
	// Sort the environment slice alphabetically to guarantee deterministic hashing
	sort.Strings(data.Environment)

	tmpl, err := template.New("unit").Parse(unitTemplate)
	if err != nil {
		return "", "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", "", err
	}
	content = buf.String()
	sum := sha256.Sum256([]byte(content))
	hash = hex.EncodeToString(sum[:])
	return content, hash, nil
}

// func (r *systemdServiceResource) sshClient(nodeAddress, nodeUserOverride string) (*ssh.Client, error) {
// 	user := r.provider.config.SSHUser.ValueString()
// 	if nodeUserOverride != "" {
// 		user = nodeUserOverride
// 	}
// 	if user == "" {
// 		return nil, fmt.Errorf("ssh_user must be set in provider or resource")
// 	}

// 	port := r.provider.config.SSHPort.ValueInt64()
// 	if port == 0 {
// 		port = 22
// 	}

// 	key := r.provider.config.SSHPrivateKey.ValueString()
// 	if key == "" {
// 		return nil, fmt.Errorf("ssh_private_key must be set in provider")
// 	}

// 	signer, err := ssh.ParsePrivateKey([]byte(key))
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to parse private key: %w", err)
// 	}

// 	config := &ssh.ClientConfig{
// 		User:            user,
// 		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
// 		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // Demo only. Use known_hosts in production.
// 	}

// 	addr := fmt.Sprintf("%s:%d", nodeAddress, port)
// 	return ssh.Dial("tcp", addr, config)
// }

func (r *systemdServiceResource) sshClient(nodeAddress, nodeUserOverride string) (*ssh.Client, error) {
	user := r.provider.config.SSHUser.ValueString()
	if nodeUserOverride != "" {
		user = nodeUserOverride
	}
	if user == "" {
		return nil, fmt.Errorf("ssh_user must be set in provider or resource")
	}

	port := r.provider.config.SSHPort.ValueInt64()
	if port == 0 {
		port = 22
	}

	key := r.provider.config.SSHPrivateKey.ValueString()
	if key == "" {
		return nil, fmt.Errorf("ssh_private_key must be set in provider")
	}

	signer, err := ssh.ParsePrivateKey([]byte(key))
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	// Locate the user's home directory to find ~/.ssh/known_hosts
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("could not find user home directory: %w", err)
	}

	knownHostsPath := filepath.Join(homeDir, ".ssh", "known_hosts")
	
	// Create the HostKeyCallback using the known_hosts file
	hostKeyCallback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load known_hosts from %s: %w", knownHostsPath, err)
	}

	config := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: hostKeyCallback,
	}

	addr := fmt.Sprintf("%s:%d", nodeAddress, port)
	return ssh.Dial("tcp", addr, config)
}

func sshExec(client *ssh.Client, cmd string) (string, string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", "", err
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	err = session.Run(cmd)
	return stdout.String(), stderr.String(), err
}

func sshWriteFile(client *ssh.Client, path, content string) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	session.Stdin = bytes.NewBufferString(content)
	cmd := fmt.Sprintf("sudo tee %s > /dev/null", path)
	return session.Run(cmd)
}

// Helper to convert the types.Map to a slice of "KEY=VALUE" strings
func convertEnvMapToList(ctx context.Context, envMap types.Map, respDiagnostics *interface{}) ([]string, bool) {
	env := make(map[string]string)
	if !envMap.IsNull() && !envMap.IsUnknown() {
		diags := envMap.ElementsAs(ctx, &env, false)
		if diags.HasError() {
			return nil, false
		}
	}
	var envList []string
	for k, v := range env {
		envList = append(envList, fmt.Sprintf("%s=%s", k, v))
	}
	return envList, true
}

func (r *systemdServiceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan systemdServiceResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var envList []string
	env := make(map[string]string)
	if !plan.Environment.IsNull() && !plan.Environment.IsUnknown() {
		diags = plan.Environment.ElementsAs(ctx, &env, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		for k, v := range env {
			envList = append(envList, fmt.Sprintf("%s=%s", k, v))
		}
	}

	content, hash, err := renderUnitFile(unitTemplateData{
		ServiceName:      plan.ServiceName.ValueString(),
		ExecStart:        plan.ExecStart.ValueString(),
		WorkingDirectory: plan.WorkingDirectory.ValueString(),
		Environment:      envList, // Pass the formatted list
	})
	if err != nil {
		resp.Diagnostics.AddError("Render Error", err.Error())
		return
	}

	client, err := r.sshClient(plan.NodeAddress.ValueString(), plan.NodeUser.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("SSH Connection Error", err.Error())
		return
	}
	defer client.Close()

	unitPath := fmt.Sprintf("/etc/systemd/system/%s.service", plan.ServiceName.ValueString())

	if err := sshWriteFile(client, unitPath, content); err != nil {
		resp.Diagnostics.AddError("Write Unit File Error", err.Error())
		return
	}

	if _, stderr, err := sshExec(client, "sudo systemctl daemon-reload"); err != nil {
		resp.Diagnostics.AddError("Daemon Reload Error", fmt.Sprintf("%s: %s", err.Error(), stderr))
		return
	}

	cmd := fmt.Sprintf("sudo systemctl enable --now %s", plan.ServiceName.ValueString())
	if _, stderr, err := sshExec(client, cmd); err != nil {
		resp.Diagnostics.AddError("Service Start Error", fmt.Sprintf("%s: %s", err.Error(), stderr))
		return
	}

	plan.UnitFileHash = types.StringValue(hash)
	plan.ID = types.StringValue(fmt.Sprintf("%s::%s", plan.NodeAddress.ValueString(), plan.ServiceName.ValueString()))

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// func (r *systemdServiceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
// 	var state systemdServiceResourceModel
// 	diags := req.State.Get(ctx, &state)
// 	resp.Diagnostics.Append(diags...)
// 	if resp.Diagnostics.HasError() {
// 		return
// 	}

// 	// ID format: node_address::service_name
// 	id := state.ID.ValueString()
// 	var nodeAddr, svcName string
	
// 	// FIX: Use strings.Split instead of fmt.Sscanf
// 	parts := strings.Split(id, "::")
// 	if len(parts) == 2 {
// 		nodeAddr = parts[0]
// 		svcName = parts[1]
// 	} else {
// 		// Fallback to stored attributes if ID parsing fails
// 		nodeAddr = state.NodeAddress.ValueString()
// 		svcName = state.ServiceName.ValueString()
// 	}

// 	client, err := r.sshClient(nodeAddr, state.NodeUser.ValueString())
// 	if err != nil {
// 		// If we cannot connect, preserve state to avoid spurious destruction
// 		resp.Diagnostics.AddWarning("SSH Read Error", err.Error())
// 		return
// 	}
// 	defer client.Close()

// 	unitPath := fmt.Sprintf("/etc/systemd/system/%s.service", svcName)
// 	stdout, _, err := sshExec(client, fmt.Sprintf("sudo cat %s 2>/dev/null || echo '__FILE_MISSING__'", unitPath))
// 	if err != nil {
// 		resp.Diagnostics.AddWarning("Read Unit File Error", err.Error())
// 		return
// 	}

// 	// FIX: Trim whitespace before checking for deletion string
// 	cleanStdout := strings.TrimSpace(stdout)
// 	if cleanStdout == "__FILE_MISSING__" || cleanStdout == "" {
// 		// Resource has been deleted externally
// 		resp.State.RemoveResource(ctx)
// 		return
// 	}

// 	// Compute hash of actual remote content using the raw stdout (not the trimmed one)
// 	sum := sha256.Sum256([]byte(stdout))
// 	actualHash := hex.EncodeToString(sum[:])

// 	state.UnitFileHash = types.StringValue(actualHash)
// 	state.NodeAddress = types.StringValue(nodeAddr)
// 	state.ServiceName = types.StringValue(svcName)

// 	diags = resp.State.Set(ctx, state)
// 	resp.Diagnostics.Append(diags...)
// }

func (r *systemdServiceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state systemdServiceResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// ID format: node_address::service_name
	id := state.ID.ValueString()
	var nodeAddr, svcName string

	parts := strings.Split(id, "::")
	if len(parts) != 2 {
		// Fail fast and loudly if the ID format is corrupted
		resp.Diagnostics.AddError(
			"Invalid Resource ID",
			fmt.Sprintf("Expected ID in the format 'nodeAddr::svcName', but got: %q", id),
		)
		return
	}
	
	nodeAddr = parts[0]
	svcName = parts[1]

	client, err := r.sshClient(nodeAddr, state.NodeUser.ValueString())
	if err != nil {
		// If we cannot connect, preserve state to avoid spurious destruction
		resp.Diagnostics.AddWarning("SSH Read Error", err.Error())
		return
	}
	defer client.Close()

	unitPath := fmt.Sprintf("/etc/systemd/system/%s.service", svcName)
	stdout, _, err := sshExec(client, fmt.Sprintf("sudo cat %s 2>/dev/null || echo '__FILE_MISSING__'", unitPath))
	if err != nil {
		resp.Diagnostics.AddWarning("Read Unit File Error", err.Error())
		return
	}

	// FIX: Trim whitespace before checking for deletion string
	cleanStdout := strings.TrimSpace(stdout)
	if cleanStdout == "__FILE_MISSING__" || cleanStdout == "" {
		// Resource has been deleted externally
		resp.State.RemoveResource(ctx)
		return
	}

	// Compute hash of actual remote content using the raw stdout (not the trimmed one)
	sum := sha256.Sum256([]byte(stdout))
	actualHash := hex.EncodeToString(sum[:])

	state.UnitFileHash = types.StringValue(actualHash)
	state.NodeAddress = types.StringValue(nodeAddr)
	state.ServiceName = types.StringValue(svcName)

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *systemdServiceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan systemdServiceResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state systemdServiceResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	plan.ID = state.ID

	var envList []string
	env := make(map[string]string)
	if !plan.Environment.IsNull() && !plan.Environment.IsUnknown() {
		diags = plan.Environment.ElementsAs(ctx, &env, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		for k, v := range env {
			envList = append(envList, fmt.Sprintf("%s=%s", k, v))
		}
	}

	content, hash, err := renderUnitFile(unitTemplateData{
		ServiceName:      plan.ServiceName.ValueString(),
		ExecStart:        plan.ExecStart.ValueString(),
		WorkingDirectory: plan.WorkingDirectory.ValueString(),
		Environment:      envList, // Pass the formatted list
	})
	if err != nil {
		resp.Diagnostics.AddError("Render Error", err.Error())
		return
	}

	client, err := r.sshClient(plan.NodeAddress.ValueString(), plan.NodeUser.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("SSH Connection Error", err.Error())
		return
	}
	defer client.Close()

	unitPath := fmt.Sprintf("/etc/systemd/system/%s.service", plan.ServiceName.ValueString())
	if err := sshWriteFile(client, unitPath, content); err != nil {
		resp.Diagnostics.AddError("Write Unit File Error", err.Error())
		return
	}

	if _, stderr, err := sshExec(client, "sudo systemctl daemon-reload"); err != nil {
		resp.Diagnostics.AddError("Daemon Reload Error", fmt.Sprintf("%s: %s", err.Error(), stderr))
		return
	}

	cmd := fmt.Sprintf("sudo systemctl restart %s", plan.ServiceName.ValueString())
	if _, stderr, err := sshExec(client, cmd); err != nil {
		resp.Diagnostics.AddError("Service Restart Error", fmt.Sprintf("%s: %s", err.Error(), stderr))
		return
	}

	plan.UnitFileHash = types.StringValue(hash)

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *systemdServiceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state systemdServiceResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	client, err := r.sshClient(state.NodeAddress.ValueString(), state.NodeUser.ValueString())
	if err != nil {
		resp.Diagnostics.AddWarning("SSH Delete Error", err.Error())
		return
	}
	defer client.Close()

	svc := state.ServiceName.ValueString()
	sshExec(client, fmt.Sprintf("sudo systemctl stop %s || true", svc))
	sshExec(client, fmt.Sprintf("sudo systemctl disable %s || true", svc))
	sshExec(client, fmt.Sprintf("sudo rm -f /etc/systemd/system/%s.service", svc))
	sshExec(client, "sudo systemctl daemon-reload")

	resp.State.RemoveResource(ctx)
}

func (r *systemdServiceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *systemdServiceResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan systemdServiceResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var envList []string
	env := make(map[string]string)
	if !plan.Environment.IsNull() && !plan.Environment.IsUnknown() {
		plan.Environment.ElementsAs(ctx, &env, false)
		for k, v := range env {
			envList = append(envList, fmt.Sprintf("%s=%s", k, v))
		}
	}

	_, expectedHash, err := renderUnitFile(unitTemplateData{
		ServiceName:      plan.ServiceName.ValueString(),
		ExecStart:        plan.ExecStart.ValueString(),
		WorkingDirectory: plan.WorkingDirectory.ValueString(),
		Environment:      envList, // Pass the formatted list
	})
	if err != nil {
		resp.Diagnostics.AddError("ModifyPlan Error", err.Error())
		return
	}

	plan.UnitFileHash = types.StringValue(expectedHash)
	diags = resp.Plan.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
}
