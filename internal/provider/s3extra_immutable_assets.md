Manages multiple immutable asset files in an S3-compatible bucket.

A common way to upload static assets for a frontend application (server-rendered or client-rendered) is to use `aws_s3_bucket_object`, `for_each`, and the `fileset` function to sync a directory of files to an S3 bucket. However, this implementation can cause downtime between two frontend deployments that use _immutable_ static assets.

Immutable assets are frontend build artifacts that use hashes in their filenames to support [long-term caching.][long-term-caching]

When deploying a new frontend build, `aws_s3_bucket_object` _deletes_ the assets of the previous build. Many users won't notice, as long as they have already loaded the assets and cached them in their browser. However, a user who navigates to a new page using the browser's [History API][history-api] (without a page reload) may encounter 404s on page-specific assets they hadn't previously encountered. Refreshing the page resolves the 404s, as the new server render points to the new deploy's assets, but this behavior still causes confusion and error.

`s3extra_immutable_assets` solves the issue by ignoring the state of previously uploaded files in S3. Instead, it determines whether or not to upload files by inspecting the hashed contents of all matching files. If the hash changes, the resource uploads the files again. The resource never deletes old files from S3.

You probably don't want unbounded growth in your S3. To limit the number of old builds in your bucket, configure lifecycle rules on the bucket itself to either move old builds to colder storage (e.g. Glacier) or expire them entirely.

`s3extra_immutable_assets` provides other advantages over the traditional `aws_s3_bucket_object` approach:

- Automatically sets `content-type` on each uploaded object by inspecting the file extension.
- Represents the set of files as a single resource instead of one resource per file. This prevents state size explosion when duplicating uploads to cross-regional buckets.

[long-term-caching]: https://developers.google.com/web/fundamentals/performance/webpack/use-long-term-caching
[history-api]: https://developer.mozilla.org/en-US/docs/Web/API/History_API