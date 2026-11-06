# Object storage

`storage/s3` is an independently versioned module. Importing the root module does not add the AWS SDK.

```go
client, err := s3.New(ctx, s3.Config{
    Bucket: "assets",
    Region: "us-east-1",
    Credentials: s3.StaticCredentials(accessKey, secret, session),
})
// check err, then defer client.Close()
err = client.Put(ctx, "folder/file.txt", body, s3.PutOptions{ContentType: "text/plain"})
object, err := client.Get(ctx, "folder/file.txt")
// caller closes object.Body
```

New requires a bucket, region and credential provider, then checks the bucket. It does not read ambient AWS credentials. Call `s3.DefaultCredentialChain()` only when that explicit chain is intended. A custom endpoint uses path-style addressing. Plaintext endpoints require `AllowPlaintext`. Certificate verification cannot be disabled through `Config.TLS`.

Put streams the body. Objects larger than the part size (default 8 MiB, maximum 64 MiB) use a multipart upload. Cancellation and other failures abort that upload. Every upload sends a SHA-256 checksum. `PutOptions` can set content type, a bounded metadata map, and server-side encryption (`AES256` or `aws:kms` with a key). Empty encryption stores the object without an encryption header. The SDK does not apply a bucket policy, object lock or retention rule.

Requests are not retried inside the client. Repeat an idempotent operation with the root retry helper when that is appropriate. Get and Head return metadata. Get's body is streaming and must be closed by the caller. Delete of a missing key succeeds. PresignGet and PresignPut reject a non-positive lifetime and any lifetime above `MaxPresign` (default 15 minutes, absolute maximum 7 days).

`AllowSmallParts` permits parts below Amazon S3's 5 MiB minimum for compatible servers. Concurrency defaults to 4 and cannot exceed 8. Close releases idle connections only when New constructed the transport.
