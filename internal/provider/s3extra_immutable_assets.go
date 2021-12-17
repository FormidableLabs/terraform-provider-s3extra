package provider

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

type s3extraImmutableAssetsType struct{}

//go:embed s3extra_immutable_assets.md
var schemaDescription string

func (t s3extraImmutableAssetsType) GetSchema(ctx context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfsdk.Schema{
		MarkdownDescription: string(schemaDescription),

		Attributes: map[string]tfsdk.Attribute{
			"bucket": {
				MarkdownDescription: "The name of the S3 bucket to upload the directory to.",
				Type:                types.StringType,
				Required:            true,

				PlanModifiers: []tfsdk.AttributePlanModifier{
					tfsdk.RequiresReplace(),
				},
			},
			"glob": {
				MarkdownDescription: "A glob of the files to recursively match on and upload to the bucket.",
				Type:                types.StringType,
				Required:            true,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.RequiresReplace(),
				},
			},
			"prefix": {
				MarkdownDescription: "A key prefix/subdirectory to place the files under.",
				Type:                types.StringType,
				Optional:            true,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.RequiresReplace(),
				},
			},
			"file_configuration": {
				MarkdownDescription: "Configuration options to apply to each matched file.",
				Attributes: tfsdk.SingleNestedAttributes(map[string]tfsdk.Attribute{
					"cache_control": {
						MarkdownDescription: "The `cache_control` header to apply to all uploaded files.",
						Type:                types.StringType,
						Optional:            true,
					},
				}),
				Optional: true,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.RequiresReplace(),
				},
			},
			"tags": {
				MarkdownDescription: "A map of tags to assign to the bucket objects. Limited to ten key-value pairs.",
				Type:                types.MapType{ElemType: types.StringType},
				Optional:            true,
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.RequiresReplace(),
				},
			},
			"id": {
				Computed:            true,
				MarkdownDescription: "A unique ID derived from the bucket, glob, and prefix attributes.",
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.UseStateForUnknown(),
					tfsdk.RequiresReplace(),
				},
				Type: types.StringType,
			},
			"file_hashes": {
				Computed:            true,
				MarkdownDescription: "A map of file paths to the hash of their contents.",
				PlanModifiers: tfsdk.AttributePlanModifiers{
					tfsdk.UseStateForUnknown(),
					tfsdk.RequiresReplace(),
				},
				Type: types.MapType{ElemType: types.StringType},
			},
		},
	}, nil
}

func (t s3extraImmutableAssetsType) NewResource(ctx context.Context, in tfsdk.Provider) (tfsdk.Resource, diag.Diagnostics) {
	provider, diags := convertProviderType(in)

	return s3extraImmutableAssets{
		provider: provider,
	}, diags
}

type fileConfiguration struct {
	CacheControl types.String `tfsdk:"cache_control"`
}

type s3extraImmutableAssetsData struct {
	Id types.String `tfsdk:"id"`

	Bucket            types.String       `tfsdk:"bucket"`
	Glob              types.String       `tfsdk:"glob"`
	Prefix            types.String       `tfsdk:"prefix"`
	FileConfiguration *fileConfiguration `tfsdk:"file_configuration"`
	Tags              map[string]string  `tfsdk:"tags"`

	FileHashes map[string]string `tfsdk:"file_hashes"`
}

type s3extraImmutableAssets struct {
	provider provider
}

// Only consider this to be a new resource if this attribute combination changes!
// S3-specific file configuration like content type and cache control can change
// successfully without "recreating" the resource.
func (r s3extraImmutableAssets) id(data *s3extraImmutableAssetsData) string {
	idHash := sha256.Sum256([]byte(
		fmt.Sprintf(
			"%s-%s-%s",
			data.Bucket.Value,
			data.Glob.Value,
			data.Prefix.Value,
		),
	))

	return hex.EncodeToString(idHash[:])
}

// `create` reads local files, uploads them to S3, and sets the ID and hashes in the provided config or state.
// Both `Create` and `Update` call this method, since we reupload everything when any attribute or hashes change.
func (r s3extraImmutableAssets) create(ctx context.Context, diags *diag.Diagnostics, data *s3extraImmutableAssetsData) {
	localFiles, err := r.provider.fileClient.ReadLocalFiles(ctx, data.Glob.Value)
	if err != nil {
		diags.AddError("Local files error", fmt.Sprintf("Failed to load local files with error: %s", err))
		return
	}

	_, err = r.provider.fileClient.UploadLocalFiles(ctx, &UploadLocalFilesInput{
		Files: localFiles,
		Configuration: FilesetConfiguration{
			Bucket:            data.Bucket.Value,
			Glob:              data.Glob.Value,
			Prefix:            data.Prefix.Value,
			FileConfiguration: data.FileConfiguration,
			Tags:              data.Tags,
		},
	})

	if err != nil {
		diags.AddError("Upload error", fmt.Sprintf("Failed to upload files to bucket with error: %s", err))
		return
	}

	data.Id = types.String{Value: r.id(data)}
	data.FileHashes = r.provider.fileClient.ToTerraformState(localFiles)
}

func (r s3extraImmutableAssets) Create(ctx context.Context, req tfsdk.CreateResourceRequest, resp *tfsdk.CreateResourceResponse) {
	var data s3extraImmutableAssetsData

	diags := req.Config.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	r.create(ctx, &diags, &data)

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
}

func (r s3extraImmutableAssets) Read(ctx context.Context, req tfsdk.ReadResourceRequest, resp *tfsdk.ReadResourceResponse) {
	var data s3extraImmutableAssetsData

	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	localFiles, err := r.provider.fileClient.ReadLocalFiles(ctx, data.Glob.Value)
	if err != nil {
		diags.AddError("Local files error", fmt.Sprintf("Failed to load local files with error: %s", err))
		return
	}

	data.Id = types.String{Value: r.id(&data)}
	data.FileHashes = r.provider.fileClient.ToTerraformState(localFiles)

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
}

func (r s3extraImmutableAssets) Update(ctx context.Context, req tfsdk.UpdateResourceRequest, resp *tfsdk.UpdateResourceResponse) {
	var plan s3extraImmutableAssetsData

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	r.create(ctx, &diags, &plan)

	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
}

// `Delete` is a no-op, as we treat the files as immutable.
func (r s3extraImmutableAssets) Delete(ctx context.Context, req tfsdk.DeleteResourceRequest, resp *tfsdk.DeleteResourceResponse) {
	var data s3extraImmutableAssetsData

	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	diags.AddWarning("Not deleting files", "This resource treats its files as immutable. To delete previously managed files, either remove them from the bucket manually or allow them to expire based on the bucket lifecycle policy.")

	resp.State.RemoveResource(ctx)
}

func (r s3extraImmutableAssets) ImportState(ctx context.Context, req tfsdk.ImportResourceStateRequest, resp *tfsdk.ImportResourceStateResponse) {
	tfsdk.ResourceImportStatePassthroughID(ctx, tftypes.NewAttributePath().WithAttributeName("id"), req, resp)
}
