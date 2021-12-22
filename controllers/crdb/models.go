package crdb

import "time"

type User struct {
	Name string `json:"name,omitempty"`
}

type SqlUser struct {
	User     User   `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
}

type SqlUserName struct {
	Name string `json:"name,omitempty"`
}

type SqlUserResponse struct {
	SqlUserName []SqlUserName `json:"users,emitempty"`
}

type ClustersResponse struct {
	Cluster []Cluster `json:"clusters,omitempty"`
}

type ClusterDetails struct {
	Cluster Cluster  `json:"cluster,omitempty"`
	Regions []Region `json:"regions,omitempty"`
}

type Cluster struct {
	ID                         string                `json:"id,omitempty"`
	Name                       string                `json:"name,omitempty"`
	CockroachVersion           string                `json:"cockroachVersion,omitempty"`
	Plan                       string                `json:"plan,omitempty"`
	CloudProvider              string                `json:"cloudProvider,omitempty"`
	State                      string                `json:"state,omitempty"`
	CreatorId                  string                `json:"creatorId,omitempty"`
	LongRunningOperationStatus string                `json:"longRunningOperationStatus,omitempty"`
	Serverless                 ClusterServerlessInfo `json:"serverless,omitempty"`
	CreatedAt                  time.Time             `json:"createdAt,omitempty"`
	UpdatedAt                  time.Time             `json:"updatedAt,omitempty"`
	DeletedAt                  time.Time             `json:"deletedAt,omitempty"`
}

type Region struct {
	Name   string `json:"name,omitempty"`
	SqlDns string `json:"sqlDns,omitempty"`
}

type ClusterServerlessInfo struct {
	Regions    []string `json:"regions,omitempty"`
	SpendLimit int      `json:"spendLimit,omitempty"`
	TenantName string   `json:"tenantName,omitempty"`
}

type Credential struct {
	OrgID  string
	APIKey string
}

type ClusterCertificate struct {
	FileName      string
	CaCertificate string
}
