package provider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/ioutil"
	"mime"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/goreleaser/fileglob"
	"golang.org/x/sync/errgroup"
)

type FileResult struct {
	path string
	hash string
	data []byte
}

type UploadLocalFilesInput struct {
	Files         map[string]FileResult
	Configuration FilesetConfiguration
}

type ChangesInputState struct {
	Files         map[string]string
	Configuration FilesetConfiguration
}

type ChangesInputIncoming struct {
	Files         map[string]FileResult
	Configuration FilesetConfiguration
}

type ChangesInput struct {
	State    *ChangesInputState
	Incoming *ChangesInputIncoming
}

type FilesetConfiguration struct {
	Bucket            string
	Glob              string
	Prefix            string
	FileConfiguration *fileConfiguration
	Tags              map[string]string
}

type FileClient struct {
	s3client *s3.Client
	uploader *manager.Uploader
}

func (c FileClient) HashLocalFiles(files map[string]FileResult) string {
	id := sha256.New()
	filesList := make([]FileResult, len(files))

	for _, file := range files {
		filesList = append(filesList, file)
	}

	sort.SliceStable(filesList, func(i, j int) bool {
		return filesList[i].hash < filesList[j].hash
	})

	for _, file := range filesList {
		id.Write(file.data)
	}

	return hex.EncodeToString(id.Sum(nil))
}

func (c FileClient) ReadLocalFiles(ctx context.Context, globPattern string) (map[string]FileResult, error) {
	paths, err := fileglob.Glob(globPattern)
	if err != nil {
		return nil, err
	}

	if len(paths) == 0 {
		return nil, errors.New("Could not find any files that match the provided glob pattern.")
	}

	group, _ := errgroup.WithContext(ctx)

	files := make(map[string]FileResult)
	lock := sync.RWMutex{}

	for _, outerPath := range paths {
		path := outerPath

		group.Go(func() error {
			data, err := ioutil.ReadFile(path)
			if err != nil {
				return err
			}

			hashBytes := sha256.Sum256(data)
			hash := hex.EncodeToString(hashBytes[:])

			lock.Lock()
			files[path] = FileResult{
				path,
				hash,
				data,
			}
			lock.Unlock()

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, err
	}

	return files, nil
}

func (c FileClient) UploadLocalFiles(ctx context.Context, input *UploadLocalFilesInput) ([]*manager.UploadOutput, error) {
	results := make([]*manager.UploadOutput, len(input.Files))

	for _, file := range input.Files {
		contentType := mime.TypeByExtension(filepath.Ext(file.path))

		key := file.path
		if len(input.Configuration.Prefix) > 0 {
			key = strings.Join([]string{input.Configuration.Prefix, file.path}, "/")
		}

		uploadInput := s3.PutObjectInput{
			Bucket:      &input.Configuration.Bucket,
			Key:         &key,
			Body:        bytes.NewReader(file.data),
			ContentType: &contentType,
		}

		if len(input.Configuration.Tags) > 0 {
			tagValues := url.Values{}
			for key, value := range input.Configuration.Tags {
				tagValues.Add(key, value)
			}

			tagQueryString := tagValues.Encode()

			uploadInput.Tagging = &tagQueryString
		}

		if input.Configuration.FileConfiguration != nil &&
			!input.Configuration.FileConfiguration.CacheControl.Null &&
			!input.Configuration.FileConfiguration.CacheControl.Unknown {
			uploadInput.CacheControl = &input.Configuration.FileConfiguration.CacheControl.Value
		}

		result, err := c.uploader.Upload(ctx, &uploadInput)
		if err != nil {
			return nil, err
		}

		// Wait for the objects to be available before returning.
		duration, err := time.ParseDuration("5m")
		if err != nil {
			return nil, err
		}
		waiter := s3.NewObjectExistsWaiter(c.s3client)
		err = waiter.Wait(ctx, &s3.HeadObjectInput{
			Bucket: &input.Configuration.Bucket,
			Key:    &key,
		}, duration)
		if err != nil {
			return nil, err
		}

		results = append(results, result)
	}

	return results, nil
}

func (c FileClient) ToTerraformState(files map[string]FileResult) map[string]string {
	hashesForPaths := make(map[string]string)

	for path, file := range files {
		hashesForPaths[path] = file.hash
	}

	return hashesForPaths
}
