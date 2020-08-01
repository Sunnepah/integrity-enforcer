//
// Copyright 2020 IBM Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package sign

import (
	"encoding/json"
	"errors"
	"fmt"

	epolpkg "github.com/IBM/integrity-enforcer/enforcer/pkg/apis/enforcepolicy/v1alpha1"
	rsigpkg "github.com/IBM/integrity-enforcer/enforcer/pkg/apis/resourcesignature/v1alpha1"
	common "github.com/IBM/integrity-enforcer/enforcer/pkg/control/common"
	logger "github.com/IBM/integrity-enforcer/enforcer/pkg/logger"
	policy "github.com/IBM/integrity-enforcer/enforcer/pkg/policy"
	"github.com/jinzhu/copier"
)

/**********************************************

				SignPolicy

***********************************************/

type SignPolicy interface {
	Eval(reqc *common.ReqContext) (*common.SignPolicyEvalResult, error)
}

type ConcreteSignPolicy struct {
	EnforcerNamespace string
	PolicyNamespace   string
	Policy            *policy.PolicyList
}

/**********************************************

				EnforceRuleStore

***********************************************/

type EnforceRuleStore interface {
	Find(reqc *common.ReqContext) *EnforceRuleList
}

type EnforceRuleStoreFromPolicy struct {
	Patterns []policy.SignerMatchPattern
}

func (self *EnforceRuleStoreFromPolicy) Find(reqc *common.ReqContext) *EnforceRuleList {
	eRules := []EnforceRule{}
	for _, p := range self.Patterns {
		r := ToPolicyRule(p)
		if _, resMatched := MatchSigner(r, reqc.GroupVersion(), reqc.Kind, reqc.Name, reqc.Namespace, nil); !resMatched {
			continue
		}
		er := &EnforceRuleFromCR{Instance: r}
		eRules = append(eRules, er)
	}
	logger.Debug("DEBUG rules: ", eRules)
	return &EnforceRuleList{Rules: eRules}
}

/**********************************************

			EnforceRule, EnforceRuleList

***********************************************/

type EnforceRule interface {
	Eval(reqc *common.ReqContext, signer *common.SignerInfo) (*EnforceRuleEvalResult, error)
}

type EnforceRuleFromCR struct {
	Instance *Rule
}

func (self *EnforceRuleFromCR) Eval(reqc *common.ReqContext, signer *common.SignerInfo) (*EnforceRuleEvalResult, error) {
	apiVersion := reqc.GroupVersion()
	kind := reqc.Kind
	name := reqc.Name
	namespace := reqc.Namespace
	var matchedRule *Rule
	var signerName string
	ruleOk, _ := MatchSigner(self.Instance, apiVersion, kind, name, namespace, signer)
	if ruleOk {
		matchedRule = self.Instance
		signerName = matchedRule.Name
	}
	result := &EnforceRuleEvalResult{
		Signer:      signer,
		SignerName:  signerName,
		Checked:     true,
		Allow:       ruleOk,
		MatchedRule: matchedRule,
		Error:       nil,
	}
	return result, nil
}

type EnforceRuleEvalResult struct {
	Signer      *common.SignerInfo
	SignerName  string
	Checked     bool
	Allow       bool
	MatchedRule *Rule
	Error       *common.CheckError
}

type EnforceRuleList struct {
	Rules []EnforceRule
}

func (self *EnforceRuleList) Eval(reqc *common.ReqContext, signer *common.SignerInfo) (*EnforceRuleEvalResult, error) {
	if len(self.Rules) == 0 {
		return &EnforceRuleEvalResult{
			Signer:     signer,
			SignerName: "",
			Allow:      true,
			Checked:    true,
		}, nil
	}
	for _, rule := range self.Rules {
		if v, err := rule.Eval(reqc, signer); err != nil {
			return v, err
		} else if v != nil && v.Allow {
			return v, nil
		}
	}
	return &EnforceRuleEvalResult{
		Allow:   false,
		Checked: true,
	}, errors.New(fmt.Sprintf("No signer policies met this resource. this resource is signed by %s", signer.Email))
}

func (self *ConcreteSignPolicy) Eval(reqc *common.ReqContext) (*common.SignPolicyEvalResult, error) {

	if reqc.IsEnforcePolicyRequest() {
		var polObj *epolpkg.EnforcePolicy
		json.Unmarshal(reqc.RawObject, &polObj)
		if ok, reasonFail := polObj.Spec.Policy.Validate(reqc, self.EnforcerNamespace, self.PolicyNamespace); !ok {
			return &common.SignPolicyEvalResult{
				Allow:   false,
				Checked: true,
				Error: &common.CheckError{
					Reason: fmt.Sprintf("Schema Error for %s; %s", common.PolicyCustomResourceKind, reasonFail),
				},
			}, nil
		}
	}

	if reqc.IsResourceSignatureRequest() {
		var rsigObj *rsigpkg.ResourceSignature
		json.Unmarshal(reqc.RawObject, &rsigObj)
		if ok, reasonFail := rsigObj.Validate(); !ok {
			return &common.SignPolicyEvalResult{
				Allow:   false,
				Checked: true,
				Error: &common.CheckError{
					Reason: fmt.Sprintf("Schema Error for %s; %s", common.SignatureCustomResourceKind, reasonFail),
				},
			}, nil
		}
	}

	// eval sign policy
	ref := reqc.ResourceRef()

	// find signature
	signStore := GetSignStore()
	rsig := signStore.GetResourceSignature(ref, reqc)
	if rsig == nil {
		return &common.SignPolicyEvalResult{
			Allow:   false,
			Checked: true,
			Error: &common.CheckError{
				Reason: "No signature found",
			},
		}, nil
	}

	// create verifier
	verifier := NewVerifier(rsig.SignType, self.EnforcerNamespace)

	// verify signature
	sigVerifyResult, err := verifier.Verify(rsig, reqc)
	if err != nil {
		return &common.SignPolicyEvalResult{
			Allow:   false,
			Checked: true,
			Error: &common.CheckError{
				Error:  err,
				Reason: "Error during signature verification",
			},
		}, nil
	}

	if sigVerifyResult == nil || sigVerifyResult.Signer == nil {
		msg := ""
		if sigVerifyResult != nil && sigVerifyResult.Error != nil {
			msg = sigVerifyResult.Error.Reason
		}
		return &common.SignPolicyEvalResult{
			Allow:   false,
			Checked: true,
			Error: &common.CheckError{
				Reason: fmt.Sprintf("Failed to verify signature; %s", msg),
			},
		}, nil
	}

	// signer
	signer := sigVerifyResult.Signer

	signerPatterns := self.Policy.GetSigner()
	logger.Debug("DEBUG patterns: ", signerPatterns)
	// get enforce rule list
	var ruleStore EnforceRuleStore = &EnforceRuleStoreFromPolicy{Patterns: signerPatterns}

	reqcForEval := makeReqcForEval(reqc, reqc.RawObject)

	ruleList := ruleStore.Find(reqcForEval)

	// evaluate enforce rules
	if ruleEvalResult, err := ruleList.Eval(reqcForEval, signer); err != nil {
		return &common.SignPolicyEvalResult{
			Signer:  signer,
			Allow:   false,
			Checked: true,
			Error: &common.CheckError{
				Error:  err,
				Reason: err.Error(),
			},
		}, nil
	} else {
		matchedPolicyStr := ""
		matchedRule := ruleEvalResult.MatchedRule
		if matchedRule != nil {
			logger.Debug("DEBUG matchedRule: ", matchedRule)
			matchedPolicy := self.Policy.FindMatchedSignerPolicy(reqc, ToSignerMatchPattern(matchedRule))
			logger.Debug("DEBUG matchedPolicy: ", matchedPolicy)
			if matchedPolicy != nil {
				matchedPolicyStr = matchedPolicy.String()
			}
		}
		return &common.SignPolicyEvalResult{
			Signer:        ruleEvalResult.Signer,
			SignerName:    ruleEvalResult.SignerName,
			Allow:         ruleEvalResult.Allow,
			Checked:       ruleEvalResult.Checked,
			MatchedPolicy: matchedPolicyStr,
			Error:         ruleEvalResult.Error,
		}, nil
	}

}

func makeReqcForEval(reqc *common.ReqContext, rawObj []byte) *common.ReqContext {
	var err error
	isResourceSignature := reqc.IsResourceSignatureRequest()

	if !isResourceSignature {
		return reqc
	}

	reqcForEval := &common.ReqContext{}
	copier.Copy(&reqcForEval, &reqc)

	if isResourceSignature {
		var rsigObj *rsigpkg.ResourceSignature
		err = json.Unmarshal(rawObj, &rsigObj)
		if err == nil {
			if rsigObj.Spec.Data[0].Metadata.Namespace != "" {
				reqcForEval.Namespace = rsigObj.Spec.Data[0].Metadata.Namespace
			}
		} else {
			logger.Error(err)
		}
	}
	return reqcForEval
}

type EnforcerPolicyType string

const (
	Unknown EnforcerPolicyType = ""
	Allow   EnforcerPolicyType = "Allow"
	Deny    EnforcerPolicyType = "Deny"
)

type Subject struct {
	Email              string `json:"email,omitempty"`
	Uid                string `json:"uid,omitempty"`
	Country            string `json:"country,omitempty"`
	Organization       string `json:"organization,omitempty"`
	OrganizationalUnit string `json:"organizationalUnit,omitempty"`
	Locality           string `json:"locality,omitempty"`
	Province           string `json:"province,omitempty"`
	StreetAddress      string `json:"streetAddress,omitempty"`
	PostalCode         string `json:"postalCode,omitempty"`
	CommonName         string `json:"commonName,omitempty"`
	SerialNumber       string `json:"serialNumber,omitempty"`
}

func (v *Subject) Match(signer *common.SignerInfo) bool {
	if signer == nil {
		return false
	}

	return policy.MatchPattern(v.Email, signer.Email) &&
		policy.MatchPattern(v.Uid, signer.Uid) &&
		policy.MatchPattern(v.Country, signer.Country) &&
		policy.MatchPattern(v.Organization, signer.Organization) &&
		policy.MatchPattern(v.OrganizationalUnit, signer.OrganizationalUnit) &&
		policy.MatchPattern(v.Locality, signer.Locality) &&
		policy.MatchPattern(v.Province, signer.Province) &&
		policy.MatchPattern(v.StreetAddress, signer.StreetAddress) &&
		policy.MatchPattern(v.PostalCode, signer.PostalCode) &&
		policy.MatchPattern(v.CommonName, signer.CommonName) &&
		policy.MatchPattern(v.SerialNumber, signer.SerialNumber)
}

type Resource struct {
	ApiVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Name       string `json:"name,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
}

type Rule struct {
	Type     EnforcerPolicyType `json:"type,omitempty"`
	Resource Resource           `json:"resource,omitempty"`
	Subject  Subject            `json:"subject,omitempty"`
	Name     string             `json:"name,omitempty"`
}

func NewSignPolicy(enforcerNamespace, policyNamespace string, policy *policy.PolicyList) (SignPolicy, error) {
	return &ConcreteSignPolicy{
		EnforcerNamespace: enforcerNamespace,
		PolicyNamespace:   policyNamespace,
		Policy:            policy,
	}, nil
}

func ToPolicyRule(self policy.SignerMatchPattern) *Rule {
	return &Rule{
		Type: Allow,
		Resource: Resource{
			ApiVersion: self.Request.ApiVersion,
			Kind:       self.Request.Kind,
			Name:       self.Request.Name,
			Namespace:  self.Request.Namespace,
		},
		Subject: Subject{
			Email:              self.Condition.Subject.Email,
			Uid:                self.Condition.Subject.Uid,
			Country:            self.Condition.Subject.Country,
			Organization:       self.Condition.Subject.Organization,
			OrganizationalUnit: self.Condition.Subject.OrganizationalUnit,
			Locality:           self.Condition.Subject.Locality,
			Province:           self.Condition.Subject.Province,
			StreetAddress:      self.Condition.Subject.StreetAddress,
			PostalCode:         self.Condition.Subject.PostalCode,
			CommonName:         self.Condition.Subject.CommonName,
			SerialNumber:       self.Condition.Subject.SerialNumber,
		},
		Name: self.Condition.Name,
	}
}

func ToSignerMatchPattern(self *Rule) policy.SignerMatchPattern {
	return policy.SignerMatchPattern{
		Request: policy.RequestMatchPattern{
			ApiVersion: self.Resource.ApiVersion,
			Kind:       self.Resource.Kind,
			Name:       self.Resource.Name,
			Namespace:  self.Resource.Namespace,
		},
		Condition: policy.SubjectCondition{
			Name: self.Name,
			Subject: policy.SubjectMatchPattern{
				Email:              self.Subject.Email,
				Uid:                self.Subject.Uid,
				Country:            self.Subject.Country,
				Organization:       self.Subject.Organization,
				OrganizationalUnit: self.Subject.OrganizationalUnit,
				Locality:           self.Subject.Locality,
				Province:           self.Subject.Province,
				StreetAddress:      self.Subject.StreetAddress,
				PostalCode:         self.Subject.PostalCode,
				CommonName:         self.Subject.CommonName,
				SerialNumber:       self.Subject.SerialNumber,
			},
		},
	}
}

func MatchSigner(r *Rule, apiVersion, kind, name, namespace string, signer *common.SignerInfo) (bool, bool) {
	apiVersionOk := policy.MatchPattern(r.Resource.ApiVersion, apiVersion)
	kindOk := policy.MatchPattern(r.Resource.Kind, kind)
	nameOk := policy.MatchPattern(r.Resource.Name, name)
	namespaceOk := policy.MatchPattern(r.Resource.Namespace, namespace)
	resourceMatched := false
	if apiVersionOk && kindOk && nameOk && namespaceOk {
		resourceMatched = true
	}
	if resourceMatched {
		if r.Subject.Match(signer) {
			return true, resourceMatched
		} else {
			return false, resourceMatched
		}
	}
	return false, resourceMatched
}
