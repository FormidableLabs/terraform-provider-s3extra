default: testacc

# Run acceptance tests
.PHONY: testacc
testacc:
	TF_ACC=1 go test ./... -v $(TESTARGS) -timeout 120m

bootstrap:
	aws cloudformation deploy \
	  --template-file support/bootstrap.yml \
	  --stack-name tfp-s3extra \
	  --parameter-overrides \
	    ServiceName=tfp-s3extra \
	    AccountNickname=prod \
	    Organization=FormidableLabs \
	    Repository=terraform-provider-s3extra \
	  --capabilities CAPABILITY_IAM CAPABILITY_NAMED_IAM
