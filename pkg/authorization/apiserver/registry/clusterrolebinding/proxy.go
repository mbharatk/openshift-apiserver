package clusterrolebinding

import (
	"context"

	metainternal "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
	rbacv1 "k8s.io/client-go/kubernetes/typed/rbac/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/kubernetes/pkg/printers"
	printerstorage "k8s.io/kubernetes/pkg/printers/storage"

	authorizationapi "github.com/openshift/openshift-apiserver/pkg/authorization/apis/authorization"
	utilregistry "github.com/openshift/openshift-apiserver/pkg/authorization/apiserver/registry/registry"
	"github.com/openshift/openshift-apiserver/pkg/authorization/apiserver/registry/util"
	authprinters "github.com/openshift/openshift-apiserver/pkg/authorization/printers/internalversion"
	authclient "github.com/openshift/openshift-apiserver/pkg/client/impersonatingclient"
)

type REST struct {
	privilegedClient restclient.Interface
	rest.TableConvertor
}

var _ rest.Lister = &REST{}
var _ rest.Getter = &REST{}
var _ rest.CreaterUpdater = &REST{}
var _ rest.GracefulDeleter = &REST{}
var _ rest.Scoper = &REST{}
var _ rest.Storage = &REST{}

func NewREST(client restclient.Interface) utilregistry.NoWatchStorage {
	return utilregistry.WrapNoWatchStorageError(&REST{
		privilegedClient: client,
		TableConvertor:   printerstorage.TableConvertor{TableGenerator: printers.NewTableGenerator().With(authprinters.AddAuthorizationOpenShiftHandler)},
	})
}

func (s *REST) New() runtime.Object {
	return &authorizationapi.ClusterRoleBinding{}
}

func (s *REST) Destroy() {}

func (s *REST) NewList() runtime.Object {
	return &authorizationapi.ClusterRoleBindingList{}
}

func (s *REST) NamespaceScoped() bool {
	return false
}

func (s *REST) List(ctx context.Context, options *metainternal.ListOptions) (runtime.Object, error) {
	client, err := s.getImpersonatingClient(ctx)
	if err != nil {
		return nil, err
	}

	optv1 := metav1.ListOptions{}
	if err := metainternal.Convert_internalversion_ListOptions_To_v1_ListOptions(options, &optv1, nil); err != nil {
		return nil, err
	}

	bindings, err := client.List(ctx, optv1)
	if err != nil {
		return nil, err
	}

	ret := &authorizationapi.ClusterRoleBindingList{ListMeta: bindings.ListMeta}
	for _, curr := range bindings.Items {
		role, err := util.ClusterRoleBindingFromRBAC(&curr)
		if err != nil {
			return nil, err
		}
		ret.Items = append(ret.Items, *role)
	}
	return ret, nil
}

func (s *REST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	client, err := s.getImpersonatingClient(ctx)
	if err != nil {
		return nil, err
	}

	ret, err := client.Get(ctx, name, *options)
	if err != nil {
		return nil, err
	}

	binding, err := util.ClusterRoleBindingFromRBAC(ret)
	if err != nil {
		return nil, err
	}
	return binding, nil
}

func (s *REST) Delete(ctx context.Context, name string, objectFunc rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	client, err := s.getImpersonatingClient(ctx)
	if err != nil {
		return nil, false, err
	}

	if err := client.Delete(ctx, name, *options); err != nil {
		return nil, false, err
	}

	return &metav1.Status{Status: metav1.StatusSuccess}, true, nil
}

func (s *REST) Create(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	client, err := s.getImpersonatingClient(ctx)
	if err != nil {
		return nil, err
	}

	convertedObj, err := util.ClusterRoleBindingToRBAC(obj.(*authorizationapi.ClusterRoleBinding))
	if err != nil {
		return nil, err
	}

	ret, err := client.Create(ctx, convertedObj, *options)
	if err != nil {
		return nil, err
	}

	binding, err := util.ClusterRoleBindingFromRBAC(ret)
	if err != nil {
		return nil, err
	}
	return binding, nil
}

func (s *REST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, _ rest.ValidateObjectFunc, _ rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	client, err := s.getImpersonatingClient(ctx)
	if err != nil {
		return nil, false, err
	}

	old, err := client.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, false, err
	}

	oldRoleBinding, err := util.ClusterRoleBindingFromRBAC(old)
	if err != nil {
		return nil, false, err
	}

	obj, err := objInfo.UpdatedObject(ctx, oldRoleBinding)
	if err != nil {
		return nil, false, err
	}

	updatedRoleBinding, err := util.ClusterRoleBindingToRBAC(obj.(*authorizationapi.ClusterRoleBinding))
	if err != nil {
		return nil, false, err
	}

	ret, err := client.Update(ctx, updatedRoleBinding, *options)
	if err != nil {
		return nil, false, err
	}

	role, err := util.ClusterRoleBindingFromRBAC(ret)
	if err != nil {
		return nil, false, err
	}
	return role, false, err
}

func (s *REST) getImpersonatingClient(ctx context.Context) (rbacv1.ClusterRoleBindingInterface, error) {
	rbacClient, err := authclient.NewImpersonatingRBACFromContext(ctx, s.privilegedClient)
	if err != nil {
		return nil, err
	}
	return rbacClient.ClusterRoleBindings(), nil
}
