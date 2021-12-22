package crdb

import (
	"errors"
	"fmt"
	"github.com/parnurzeal/gorequest"
	"net/http"
	"time"
)

const (
	crdbUrlPrefix  = "https://cockroachlabs.cloud/api/v1/orgs/%s/clusters"
	clusterCertUrl = "https://cockroachlabs.cloud/clusters/%v/cert"
)

type ClusterService interface {
	ListClusters() ([]Cluster, *http.Response, error)
	GetCluster(clusterID string) (*ClusterDetails, *http.Response, error)
	ListUsers(clusterID string) ([]SqlUser, *http.Response, error)
	CreateUser(clusterID string) (*SqlUser, *http.Response, error)
	DeleteUser(clusterID string, userName string) (*http.Response, error)
	GetClusterCertificate(clusterID string) (*ClusterCertificate, *http.Response, error)
	SetCredential(cred *Credential)
}

type ClusterServiceCloud struct {
	cred *Credential
	req  *gorequest.SuperAgent
}

func (c ClusterServiceCloud) setAuthHeader() {
	c.req.Set("Accept", "application/json")
	c.req.Set("Authorization", fmt.Sprintf("Bearer %s", c.cred.APIKey))
}

func NewClusterServiceCloud(cred *Credential) *ClusterServiceCloud {
	return &ClusterServiceCloud{
		cred: cred,
		req:  gorequest.New(),
	}
}

func (c ClusterServiceCloud) SetCredential(cred *Credential) {
	c.cred = cred
}

func wrapErrors(errs []error) error {
	allErrs := errors.New("")
	for _, e := range errs {
		allErrs = fmt.Errorf(allErrs.Error()+" %w", e)
	}
	return allErrs
}

func (c ClusterServiceCloud) ListClusters() ([]Cluster, *http.Response, error) {
	url := fmt.Sprintf(crdbUrlPrefix+"?active=true", c.cred.OrgID)
	c.req.Get(url)
	c.setAuthHeader()

	cluResp := new(ClustersResponse)
	httpRep, body, errs := c.req.EndStruct(cluResp)
	if len(errs) > 0 {
		return nil, httpRep, wrapErrors(errs)
	}
	if httpRep.StatusCode != http.StatusOK {
		return nil, httpRep, errors.New(string(body))
	}
	return cluResp.Cluster, httpRep, nil
}

func (c ClusterServiceCloud) GetCluster(clusterID string) (*ClusterDetails, *http.Response, error) {
	url := fmt.Sprintf(crdbUrlPrefix+"/%v", c.cred.OrgID, clusterID)
	c.req.Get(url)
	c.setAuthHeader()

	cluResp := new(ClusterDetails)
	httpRep, body, errs := c.req.EndStruct(cluResp)
	if len(errs) > 0 {
		return nil, httpRep, wrapErrors(errs)
	}
	if httpRep.StatusCode != http.StatusOK {
		return nil, httpRep, errors.New(string(body))
	}
	return cluResp, httpRep, nil
}

func (c ClusterServiceCloud) ListUsers(clusterID string) ([]SqlUser, *http.Response, error) {
	url := fmt.Sprintf(crdbUrlPrefix+"/%v/sql-users", c.cred.OrgID, clusterID)
	c.req.Get(url)
	c.setAuthHeader()

	usrResp := new(SqlUserResponse)
	httpRep, body, errs := c.req.EndStruct(usrResp)
	if len(errs) > 0 {
		return nil, httpRep, wrapErrors(errs)
	}
	if httpRep.StatusCode != http.StatusOK {
		return nil, httpRep, errors.New(string(body))
	}
	var sqlUser []SqlUser
	for _, u := range usrResp.SqlUserName {
		sqlUser = append(sqlUser, SqlUser{
			User: User{Name: u.Name},
		})
	}
	return sqlUser, httpRep, nil
}

func (c ClusterServiceCloud) CreateUser(clusterID string) (*SqlUser, *http.Response, error) {
	url := fmt.Sprintf(crdbUrlPrefix+"/%v/sql-users", c.cred.OrgID, clusterID)

	sqlUserName := fmt.Sprintf("sql_user_%v", time.Now().UnixNano())
	sqlUserPwd := generatePassword()

	c.req.Post(url)
	c.setAuthHeader()
	aUser := SqlUser{
		User: User{
			Name: sqlUserName,
		},
		Password: sqlUserPwd,
	}
	c.req.Header.Set("Content-Type", "application/json")
	resp, body, errs := c.req.Send(aUser).End()
	if len(errs) > 0 {
		return nil, nil, wrapErrors(errs)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, resp, errors.New(body)
	}
	return &aUser, resp, nil
}

func (c ClusterServiceCloud) DeleteUser(clusterID string, userName string) (*http.Response, error) {
	url := fmt.Sprintf(crdbUrlPrefix+"/%v/sql-users/%v", c.cred.OrgID, clusterID, userName)

	c.req.Delete(url)
	c.setAuthHeader()
	resp, body, errs := c.req.End()
	if len(errs) > 0 {
		return resp, wrapErrors(errs)
	}
	if resp.StatusCode != http.StatusOK {
		return resp, errors.New(body)
	}
	return resp, nil
}

func (c ClusterServiceCloud) GetClusterCertificate(clusterID string) (*ClusterCertificate, *http.Response, error) {
	url := fmt.Sprintf(clusterCertUrl, clusterID)
	c.req.Get(url)

	httpRep, body, errs := c.req.End()
	if len(errs) > 0 {
		return nil, httpRep, wrapErrors(errs)
	}
	if httpRep.StatusCode != http.StatusOK {
		return nil, httpRep, errors.New(body)
	}
	caCert := &ClusterCertificate{
		FileName:      "root.crt",
		CaCertificate: body,
	}
	return caCert, httpRep, nil
}
