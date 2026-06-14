package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/apigateway"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	awslambda "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/lambda"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

const (
	stageName        = "api"
	localEndpointURL = "http://localhost:4566"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "gohtmxelm-edge-demo")
		artifact := cfg.Get("lambdaArtifact")
		if artifact == "" {
			artifact = "../dist/edge-datastar-lambda.zip"
		}
		hash, err := fileBase64SHA256(artifact)
		if err != nil {
			return err
		}

		awsCfg := config.New(ctx, "aws")
		region := awsCfg.Get("region")
		if region == "" {
			region = "eu-west-1"
		}

		tags := pulumi.StringMap{
			"Tier":           pulumi.String("Local"),
			"BusinessUnit":   pulumi.String("DevOps"),
			"Description":    pulumi.String("gohtmxelm Datastar edge SSE local stack"),
			"TechnicalOwner": pulumi.String("dev@local"),
		}

		role, err := iam.NewRole(ctx, "edge-datastar-lambda-role", &iam.RoleArgs{
			Name:             pulumi.String("gohtmxelm-edge-datastar-lambda"),
			AssumeRolePolicy: pulumi.String(assumeRolePolicy()),
			Tags:             tags,
		})
		if err != nil {
			return err
		}

		if _, err := iam.NewRolePolicy(ctx, "edge-datastar-lambda-logs", &iam.RolePolicyArgs{
			Role:   role.ID(),
			Policy: pulumi.String(lambdaLogsPolicy()),
		}); err != nil {
			return err
		}

		fn, err := awslambda.NewFunction(ctx, "edge-datastar-stream", &awslambda.FunctionArgs{
			Name:           pulumi.String("gohtmxelm-edge-datastar-stream"),
			Description:    pulumi.String("Streams Datastar SSE patches through API Gateway"),
			Runtime:        pulumi.String("provided.al2023"),
			Handler:        pulumi.String("bootstrap"),
			Architectures:  pulumi.StringArray{pulumi.String("arm64")},
			Role:           role.Arn,
			Code:           pulumi.NewFileArchive(artifact),
			SourceCodeHash: pulumi.String(hash),
			MemorySize:     pulumi.Int(128),
			Timeout:        pulumi.Int(30),
			Tags:           tags,
		})
		if err != nil {
			return err
		}

		api, err := apigateway.NewRestApi(ctx, "edge-datastar-api", &apigateway.RestApiArgs{
			Name:        pulumi.String("gohtmxelm-edge-datastar-api"),
			Description: pulumi.String("Same-origin /api/* Datastar SSE demo"),
			EndpointConfiguration: &apigateway.RestApiEndpointConfigurationArgs{
				Types: pulumi.String("REGIONAL"),
			},
			Tags: tags,
		})
		if err != nil {
			return err
		}

		edgeResource, err := apigateway.NewResource(ctx, "edge-datastar-resource", &apigateway.ResourceArgs{
			RestApi:  api.ID(),
			ParentId: api.RootResourceId,
			PathPart: pulumi.String("edge-datastar"),
		})
		if err != nil {
			return err
		}

		streamResource, err := apigateway.NewResource(ctx, "edge-datastar-stream-resource", &apigateway.ResourceArgs{
			RestApi:  api.ID(),
			ParentId: edgeResource.ID(),
			PathPart: pulumi.String("stream"),
		})
		if err != nil {
			return err
		}

		method, err := apigateway.NewMethod(ctx, "edge-datastar-stream-get", &apigateway.MethodArgs{
			RestApi:       api.ID(),
			ResourceId:    streamResource.ID(),
			HttpMethod:    pulumi.String("GET"),
			Authorization: pulumi.String("NONE"),
		})
		if err != nil {
			return err
		}

		integrationURI := pulumi.Sprintf(
			"arn:aws:apigateway:%s:lambda:path/2015-03-31/functions/%s/invocations",
			region,
			fn.Arn,
		)
		integration, err := apigateway.NewIntegration(ctx, "edge-datastar-stream-integration", &apigateway.IntegrationArgs{
			RestApi:               api.ID(),
			ResourceId:            streamResource.ID(),
			HttpMethod:            method.HttpMethod,
			IntegrationHttpMethod: pulumi.String("POST"),
			Type:                  pulumi.String("AWS_PROXY"),
			Uri:                   integrationURI,
			TimeoutMilliseconds:   pulumi.Int(29000),
		})
		if err != nil {
			return err
		}

		permission, err := awslambda.NewPermission(ctx, "edge-datastar-apigw-permission", &awslambda.PermissionArgs{
			Action:    pulumi.String("lambda:InvokeFunction"),
			Function:  fn.Name,
			Principal: pulumi.String("apigateway.amazonaws.com"),
			SourceArn: pulumi.Sprintf("%s/*/GET/edge-datastar/stream", api.ExecutionArn),
		})
		if err != nil {
			return err
		}

		deployment, err := apigateway.NewDeployment(ctx, "edge-datastar-deployment", &apigateway.DeploymentArgs{
			RestApi: api.ID(),
			Triggers: pulumi.StringMap{
				"artifact": pulumi.String(hash),
				"route":    integration.ID(),
			},
		}, pulumi.DependsOn([]pulumi.Resource{integration, permission}), pulumi.RetainOnDelete(true))
		if err != nil {
			return err
		}

		stage, err := apigateway.NewStage(ctx, "edge-datastar-stage", &apigateway.StageArgs{
			RestApi:    api.ID(),
			Deployment: deployment.ID(),
			StageName:  pulumi.String(stageName),
			Tags:       tags,
		})
		if err != nil {
			return err
		}

		apiURL := pulumi.Sprintf("%s/restapis/%s/%s/_user_request_", localEndpointURL, api.ID(), stage.StageName)
		streamURL := pulumi.Sprintf("%s/edge-datastar/stream", apiURL)
		patchCommand := pulumi.String("floci local stack uses Lambda proxy /invocations; responseTransferMode=STREAM is only needed for the response-streaming entrypoint in AWS.")

		ctx.Export("edge:restApiId", api.ID())
		ctx.Export("edge:stageName", stage.StageName)
		ctx.Export("edge:resourceId", streamResource.ID())
		ctx.Export("edge:localInvokeUrl", streamURL)
		ctx.Export("edge:sameOriginPath", pulumi.String("/api/edge-datastar/stream"))
		ctx.Export("edge:streamingPatchCommand", patchCommand)
		ctx.Export("starbase:SUBSPACE_ORIGIN", apiURL)
		return nil
	})
}

func fileBase64SHA256(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read lambda artifact %q: %w", path, err)
	}
	sum := sha256.Sum256(b)
	return base64.StdEncoding.EncodeToString(sum[:]), nil
}

func assumeRolePolicy() string {
	doc := map[string]any{
		"Version": "2012-10-17",
		"Statement": []map[string]any{{
			"Effect": "Allow",
			"Principal": map[string]string{
				"Service": "lambda.amazonaws.com",
			},
			"Action": "sts:AssumeRole",
		}},
	}
	b, _ := json.Marshal(doc)
	return string(b)
}

func lambdaLogsPolicy() string {
	doc := map[string]any{
		"Version": "2012-10-17",
		"Statement": []map[string]any{{
			"Effect": "Allow",
			"Action": []string{
				"logs:CreateLogGroup",
				"logs:CreateLogStream",
				"logs:PutLogEvents",
			},
			"Resource": "*",
		}},
	}
	b, _ := json.Marshal(doc)
	return string(b)
}
