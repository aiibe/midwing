package main

import "encoding/json"

// Schema describes the input contract; the shared compiler enforces cross-field rules.
func serviceSchema() any {
	var schema any
	if err := json.Unmarshal([]byte(serviceSchemaJSON), &schema); err != nil {
		panic(err)
	}
	return schema
}

const serviceSchemaJSON = `{
  "$schema":"https://json-schema.org/draft/2020-12/schema",
  "title":"Midwing custom service",
  "type":"object",
  "additionalProperties":false,
  "required":["id","name","baseUrl","authKind","header","endpoints"],
  "properties":{
    "id":{"type":"string","pattern":"^[a-z][a-z0-9-]{0,47}$","description":"Unique service ID; schema, create, validate, and update are reserved."},
    "name":{"type":"string","minLength":1,"maxLength":80},
    "baseUrl":{"type":"string","format":"uri","pattern":"^https://","description":"HTTPS URL, optional base path; no credentials, queries, fragments, or traversal."},
    "authKind":{"enum":["token","password"]},
    "header":{"type":"string","minLength":1,"maxLength":80,"description":"Credential header, e.g. Authorization or X-API-Key; transport headers and Cookie are prohibited."},
    "prefix":{"type":"string","maxLength":80,"default":"","description":"Printable ASCII; use Bearer followed by a space for bearer tokens. Omitted means an empty prefix."},
    "password":{
      "type":"object","additionalProperties":false,
      "required":["loginPath","emailField","passwordField","tokenField"],
      "properties":{
        "loginPath":{"type":"string","description":"Relative JSON POST login path, appended to baseUrl."},
        "emailField":{"type":"string","pattern":"^[A-Za-z_][A-Za-z0-9_]*$"},
        "passwordField":{"type":"string","pattern":"^[A-Za-z_][A-Za-z0-9_]*$","description":"Must differ from emailField."},
        "tokenField":{"type":"string","pattern":"^[A-Za-z_][A-Za-z0-9_]*(\\.[A-Za-z_][A-Za-z0-9_]*)*$","description":"Dotted JSON response path."},
        "refreshPath":{"type":"string","default":"","description":"Optional JSON POST refresh path using the current credential header; same tokenField. Empty means no refresh."}
      }
    },
    "endpoints":{
      "type":"array","minItems":1,"maxItems":100,
      "items":{
        "type":"object","additionalProperties":false,"required":["path"],
        "properties":{
          "description":{"type":"string","maxLength":500,"default":"","description":"Permission description; blank or omitted means Read followed by the endpoint path."},
          "path":{"type":"string","maxLength":1024,"description":"Allowed GET path: exact or unique segment placeholders such as /items/{id}; endpoint patterns must not overlap; no queries, traversal, or login/refresh paths."},
          "queryParameters":{"type":"array","maxItems":30,"uniqueItems":true,"default":[],"items":{"type":"string","pattern":"^[A-Za-z_][A-Za-z0-9_]*$"},"description":"Omitted means no allowed queries. Values are strings, one per key, at most 2048 characters."}
        }
      }
    }
  },
  "allOf":[
    {"if":{"properties":{"authKind":{"const":"password"}}},"then":{"required":["password"]}},
    {"if":{"properties":{"authKind":{"const":"token"}}},"then":{"not":{"required":["password"]}}}
  ],
  "examples":[
    {"id":"example-token","name":"Example Token","baseUrl":"https://api.example.com/v1","authKind":"token","header":"Authorization","prefix":"Bearer ","endpoints":[{"path":"/items","queryParameters":["limit","cursor"]},{"path":"/items/{id}"}]},
    {"id":"example-password","name":"Example Password","baseUrl":"https://api.example.com/v1","authKind":"password","header":"Authorization","prefix":"Bearer ","password":{"loginPath":"/login","emailField":"email","passwordField":"password","tokenField":"session.token","refreshPath":"/refresh"},"endpoints":[{"path":"/items"}]}
  ],
  "description":"One JSON object, max 1 MiB, with no credentials. Defaults describe omission behavior; other fields are not inferred. Run services validate FILE|- for authoritative cross-field validation. Validation does not check ID availability. Create the definition, then connect and grant permissions in the desktop app."
}`
