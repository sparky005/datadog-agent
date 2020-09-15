// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

/*
Package api implements the agent IPC api. Using HTTP
calls, it's possible to communicate with the agent,
sending commands and receiving infos.
*/
package api

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/DataDog/datadog-agent/cmd/agent/api/pb"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	hostutil "github.com/DataDog/datadog-agent/pkg/util"
)

type server struct {
	pb.UnimplementedAgentServer
}

type serverSecure struct {
	pb.UnimplementedAgentServer

	// TODO(juliogreff): tagger.Tagger is a concrete type that makes
	// testing harder than it should be. We should make that concrete type
	// private, and create a new tagger.Tagger interface that replicates
	// it.,
	tagger *tagger.Tagger
}

func (s *server) GetHostname(ctx context.Context, in *pb.HostnameRequest) (*pb.HostnameReply, error) {
	h, err := hostutil.GetHostname()
	if err != nil {
		return &pb.HostnameReply{}, err
	}
	return &pb.HostnameReply{Hostname: h}, nil
}

// AuthFuncOverride implements the `grpc_auth.ServiceAuthFuncOverride` interface which allows
// override of the AuthFunc registered with the unary interceptor.
//
// see: https://godoc.org/github.com/grpc-ecosystem/go-grpc-middleware/auth#ServiceAuthFuncOverride
func (s *server) AuthFuncOverride(ctx context.Context, fullMethodName string) (context.Context, error) {
	return ctx, nil
}

func (s *serverSecure) StreamTags(in *pb.StreamTagsRequest, out pb.AgentSecure_StreamTagsServer) error {
	cardinality, err := pb2taggerCardinality(in.Cardinality)
	if err != nil {
		return err
	}

	// TODO(juliogreff): implement filtering

	// TODO(juliogreff): list then watch is a massive race condition that I
	// need to fix :)
	entities := s.tagger.List(cardinality)
	for id, entity := range entities.Entities {
		entityID, err := tagger2pbEntityID(id)
		if err != nil {
			// TODO(juliogreff): log and continue
			continue
		}

		err = out.Send(&pb.StreamTagsResponse{
			Type: pb.EventType_ADDED,
			Entity: &pb.Entity{
				Id:   entityID,
				Tags: entity.Tags,
			},
		})
		if err != nil {
			return err
		}
	}

	c := s.tagger.Subscribe()
	for event := range c {
		entity := event.Entity
		entityID, err := tagger2pbEntityID(entity.ID)
		if err != nil {
			// TODO(juliogreff): log and continue
			continue
		}

		var eventType pb.EventType
		switch event.EventType {
		case tagger.EventTypeAdd:
			eventType = pb.EventType_ADDED
		case tagger.EventTypeModify:
			eventType = pb.EventType_MODIFIED
		case tagger.EventTypeRemove:
			eventType = pb.EventType_DELETED
		default:
			// TODO(juliogreff): log and continue
			continue
		}

		err = out.Send(&pb.StreamTagsResponse{
			Type: eventType,
			Entity: &pb.Entity{
				Id:   entityID,
				Tags: entity.Tags,
			},
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *serverSecure) FetchEntity(ctx context.Context, in *pb.FetchEntityRequest) (*pb.Entity, error) {
	entityID := fmt.Sprintf("%s://%s", in.Id.Prefix, in.Id.Uid)
	cardinality, err := pb2taggerCardinality(in.Cardinality)
	if err != nil {
		return nil, err
	}

	tags, err := s.tagger.Tag(entityID, cardinality)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s", err)
	}

	return &pb.Entity{
		Id:   in.Id,
		Tags: tags,
	}, nil
}

func tagger2pbEntityID(entityID string) (*pb.EntityId, error) {
	parts := strings.SplitN(entityID, "://", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid entity id %q", entityID)
	}

	return &pb.EntityId{
		Prefix: parts[0],
		Uid:    parts[1],
	}, nil
}

func pb2taggerCardinality(pbCardinality pb.TagCardinality) (collectors.TagCardinality, error) {
	switch pbCardinality {
	case pb.TagCardinality_LOW:
		return collectors.LowCardinality, nil
	case pb.TagCardinality_ORCHESTRATOR:
		return collectors.OrchestratorCardinality, nil
	case pb.TagCardinality_HIGH:
		return collectors.HighCardinality, nil
	}

	return 0, status.Errorf(codes.InvalidArgument, "invalid cardinality %q", pbCardinality)
}
