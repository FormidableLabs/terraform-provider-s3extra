package provider

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"golang.org/x/sync/errgroup"
)

func TestAccS3ExtraImmutableAssets(t *testing.T) {
	ctx := context.TODO()
	config, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	s3client := s3.NewFromConfig(config)

	bucketName := fmt.Sprintf("s3extra-acc-%s", acctest.RandStringFromCharSet(16, acctest.CharSetAlphaNum))
	bucket := aws.String(bucketName)

	_, err = s3client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: bucket,
		ACL:    types.BucketCannedACLPrivate,
	})

	duration, err := time.ParseDuration("5m")
	if err != nil {
		t.Fatal(err.Error())
	}
	waiter := s3.NewBucketExistsWaiter(s3client)
	err = waiter.Wait(ctx, &s3.HeadBucketInput{Bucket: bucket}, duration)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Empty and delete the bucket after running the tests.
	t.Cleanup(func() {
		input := &s3.ListObjectsV2Input{Bucket: bucket}

		for {
			objects, err := s3client.ListObjectsV2(ctx, input)
			if err != nil {
				t.Fatal(err.Error())
			}

			for _, item := range objects.Contents {
				_, err := s3client.DeleteObject(ctx, &s3.DeleteObjectInput{
					Bucket: bucket,
					Key:    item.Key,
				})
				if err != nil {
					t.Fatal(err.Error())
				}
			}

			if objects.IsTruncated {
				input.ContinuationToken = objects.ContinuationToken
			} else {
				break
			}
		}

		_, err = s3client.DeleteBucket(ctx, &s3.DeleteBucketInput{
			Bucket: bucket,
		})
		if err != nil {
			t.Fatal(err.Error())
		}
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccS3ExtraImmutableAssetsConfig(t, attributes{
					Bucket:       bucketName,
					Glob:         "**/*.{txt,js}",
					CacheControl: "max-age=0",
					Prefix:       "",
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("s3extra_immutable_assets.test", "bucket", bucketName),
					resource.TestCheckResourceAttr("s3extra_immutable_assets.test", "glob", "**/*.{txt,js}"),
					resource.TestCheckResourceAttr("s3extra_immutable_assets.test", "file_configuration.cache_control", "max-age=0"),

					// Should not apply a default prefix
					resource.TestCheckNoResourceAttr("s3extra_immutable_assets.test", "prefix"),

					testAccS3ExtraImmutableAssetsCheckFilesExist(s3client, "s3extra_immutable_assets.test", map[string]fileAssertion{
						"fixtures/hello.txt": {
							ContentType: "text/plain; charset=utf-8",
						},
						"fixtures/static/main.js": {
							ContentType: "application/javascript",
						},
					}),
				),
			},
			// Prefix changes should recreate the resource
			{
				Config: testAccS3ExtraImmutableAssetsConfig(t, attributes{
					Bucket:       bucketName,
					Glob:         "**/*.{txt,js}",
					CacheControl: "max-age=0",
					Prefix:       "updated",
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("s3extra_immutable_assets.test", "prefix", "updated"),
					testAccS3ExtraImmutableAssetsCheckFilesExist(s3client, "s3extra_immutable_assets.test", map[string]fileAssertion{
						"fixtures/hello.txt": {
							ContentType: "text/plain; charset=utf-8",
						},
						"fixtures/static/main.js": {
							ContentType: "application/javascript",
						},
					}),
				),
			},
			// Glob changes should recreate the resource
			{
				Config: testAccS3ExtraImmutableAssetsConfig(t, attributes{
					Bucket:       bucketName,
					Glob:         "**/*.txt",
					CacheControl: "max-age=0",
					Prefix:       "updated",
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("s3extra_immutable_assets.test", "prefix", "updated"),
					testAccS3ExtraImmutableAssetsCheckFilesExist(s3client, "s3extra_immutable_assets.test", map[string]fileAssertion{
						"fixtures/hello.txt": {
							ContentType: "text/plain; charset=utf-8",
						},
					}),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

type attributes struct {
	Bucket       string
	Glob         string
	Prefix       string
	CacheControl string
}

func testAccS3ExtraImmutableAssetsConfig(t *testing.T, attributes attributes) string {
	tmpl := template.Must(template.New("config").Parse(`
terraform {
  required_version = ">= 1.0.3"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "3.69.0"
    }
  }

  backend "s3" {
  	region = "us-east-1"

  	bucket = "tfp-s3extra-prod-state"
  	key    = "acc-test/terraform.tfstate"

  	dynamodb_table = "tfp-s3extra-prod-locks"
  }
}

resource "s3extra_immutable_assets" "test" {
	bucket = "{{.Bucket}}"
	glob = "{{.Glob}}"
	{{if .Prefix}}
	prefix = "{{.Prefix}}"
	{{end}}
	file_configuration = {
		cache_control = "{{.CacheControl}}"
	}
	tags = {
		hello = "world"
	}
}
`))

	var config bytes.Buffer
	err := tmpl.Execute(&config, attributes)
	if err != nil {
		t.Fatal(err)
	}

	return config.String()
}

type fileAssertion struct {
	ContentType string
}

func testAccS3ExtraImmutableAssetsCheckFilesExist(s3client *s3.Client, resourceName string, assertions map[string]fileAssertion) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := context.TODO()

		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("Not found: %s", resourceName)
		}

		for _, attr := range []string{"bucket", "glob"} {
			if _, ok := rs.Primary.Attributes[attr]; !ok {
				return fmt.Errorf("Missing required attribute: %s", attr)
			}
		}

		group, _ := errgroup.WithContext(ctx)

		for outerKey, outerAssertion := range assertions {
			key := outerKey
			assertion := outerAssertion
			if prefix, ok := rs.Primary.Attributes["prefix"]; ok {
				key = strings.Join([]string{prefix, key}, "/")
			}

			group.Go(func() error {
				result, err := s3client.GetObject(ctx, &s3.GetObjectInput{
					Bucket: aws.String(rs.Primary.Attributes["bucket"]),
					Key:    aws.String(key),
				})
				if err != nil {
					return err
				}

				if *result.ContentType != assertion.ContentType {
					return fmt.Errorf("Uploaded file content type does not match the expected inferred content type. Expected: %s, found: %s", assertion.ContentType, *result.ContentType)
				}

				if cacheControl, ok := rs.Primary.Attributes["file_configuration.cache_control"]; ok {
					if *result.CacheControl != cacheControl {
						return fmt.Errorf("Uploaded file cache control does not match the one in state. Expected: %s, found: %s", cacheControl, *result.CacheControl)
					}
				}

				return nil
			})
		}

		if err := group.Wait(); err != nil {
			return err
		}

		return nil
	}
}
