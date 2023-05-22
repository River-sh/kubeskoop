package nodemanager

import (
	"fmt"
	"sync"

	"github.com/alibaba/kubeskoop/pkg/skoop/collector"
	ctx "github.com/alibaba/kubeskoop/pkg/skoop/context"
	"github.com/alibaba/kubeskoop/pkg/skoop/k8s"
	"github.com/alibaba/kubeskoop/pkg/skoop/model"
	"github.com/alibaba/kubeskoop/pkg/skoop/plugin"

	"k8s.io/client-go/kubernetes"
)

type NetNodeManager interface {
	GetNetNodeFromID(nodeType model.NetNodeType, id string) (model.NetNodeAction, error)
}

type defaultNetNodeManager struct {
	parent           NetNodeManager
	client           *kubernetes.Clientset
	ipCache          *k8s.IPCache
	plugin           plugin.Plugin
	collectorManager collector.Manager
	cache            sync.Map
}

func NewNetNodeManager(ctx *ctx.Context, networkPlugin plugin.Plugin, collectorManager collector.Manager) (NetNodeManager, error) {
	return &defaultNetNodeManager{
		client:           ctx.KubernetesClient(),
		ipCache:          ctx.ClusterConfig().IPCache,
		plugin:           networkPlugin,
		collectorManager: collectorManager,
	}, nil
}

func NewNetNodeManagerWithParent(ctx *ctx.Context, parent NetNodeManager, networkPlugin plugin.Plugin, collectorManager collector.Manager) (NetNodeManager, error) {
	return &defaultNetNodeManager{
		parent:           parent,
		client:           ctx.KubernetesClient(),
		ipCache:          ctx.ClusterConfig().IPCache,
		plugin:           networkPlugin,
		collectorManager: collectorManager,
	}, nil
}

func (m *defaultNetNodeManager) GetNetNodeFromID(nodeType model.NetNodeType, id string) (model.NetNodeAction, error) {
	key := m.cacheKey(nodeType, id)
	if node, ok := m.cache.Load(key); ok {
		return node.(model.NetNodeAction), nil
	}

	var ret model.NetNodeAction

	switch nodeType {
	case model.NetNodeTypePod:
		k8sPod, err := m.ipCache.GetPodFromIP(id)
		if err != nil {
			return nil, err
		}

		if k8sPod == nil {
			return nil, fmt.Errorf("k8s pod not found from ip %s", id)
		}

		podInfo, err := m.collectorManager.CollectPod(k8sPod.Namespace, k8sPod.Name)
		if err != nil {
			return nil, fmt.Errorf("error run collector for pod: %v", err)
		}

		ret, err = m.plugin.CreatePod(podInfo)
		if err != nil {
			return nil, fmt.Errorf("error create pod: %v", err)
		}
	case model.NetNodeTypeNode:
		nodeInfo, err := m.collectorManager.CollectNode(id)
		if err != nil {
			return nil, fmt.Errorf("error run collector for node: %v", err)
		}

		ret, err = m.plugin.CreateNode(nodeInfo)
		if err != nil {
			return nil, fmt.Errorf("error create node: %v", err)
		}
	default:
		if m.parent != nil {
			var err error
			ret, err = m.parent.GetNetNodeFromID(nodeType, id)
			if err != nil {
				return nil, err
			}
		} else {
			ret = &plugin.GenericNetNode{
				NetNode: &model.NetNode{
					Type:    model.NetNodeTypeGeneric,
					ID:      id,
					Actions: map[*model.Link]*model.Action{},
				},
			}
		}
	}

	if ret != nil {
		m.cache.Store(key, ret)
	}
	return ret, nil
}

func (m *defaultNetNodeManager) cacheKey(typ model.NetNodeType, id string) string {
	return fmt.Sprintf("%s---%s", typ, id)
}

var _ NetNodeManager = &defaultNetNodeManager{}
