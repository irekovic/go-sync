# Simple Sync Tool

You can point it to a local folder with -f flag. It will monitor that folder and every file found there
it will try to sync to cloud. - Updates and deletes also.

Destination is given by -url flag, witch works in combination with environment variables to support 
various cloud providers. Currently supported ones are:

## Azure Blob Storage

URL for targeting Azure Blob:

    azblob://my-container

Credentials are found in the environment variables
`AZURE_STORAGE_ACCOUNT` plus at least one of `AZURE_STORAGE_KEY`
and `AZURE_STORAGE_SAS_TOKEN`.

## Local File System

You can use local folder instead of cloud storage if you use file:// url.
You will have to provide absolute path, and path MUST exist.

## Google Cloud Storage

You can use Google Cloud Storage with url that looks like this:

    gs://my-bucket

Before being able to access GCS you have to login by executing:

     gcloud auth login

The following query parameters are supported:

    - access_id: sets Options.GoogleAccessID
    - private_key_path: path to read for Options.PrivateKey

## AWS S3

For AWS S3 URL must contain region where bucket is located like this:

    s3://my-bucket?region=us-west-1

And credentials can be provided in following environment variables:

    # Access Key ID
    AWS_ACCESS_KEY_ID=AKID
    AWS_ACCESS_KEY=AKID # only read if AWS_ACCESS_KEY_ID is not set.

    # Secret Access Key
    AWS_SECRET_ACCESS_KEY=SECRET
    AWS_SECRET_KEY=SECRET=SECRET # only read if AWS_SECRET_ACCESS_KEY is not set.

    # Session Token
    AWS_SESSION_TOKEN=TOKEN

    # Optionaly you can provide region in environment also
    AWS_REGION=us-east-1

# Example

    go-sync -f folder/to/monitor -url:file:///C:/Repo/

Will monitor folder/to/monitor and sync it's content to local folder on windows C:\\Repo\.

    AZURE_STORAGE_ACCOUNT=xxx AZURE_STORAGE_KEY=yyy go-sync -f c:\\monitored_dir -url:azureblob://az-repo/

Will monitor C:\\monitored_dir and sync it with container az-repo that is located on Azure Cloud.