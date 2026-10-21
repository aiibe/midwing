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
    "headers":{
      "type":"object","default":{},
      "propertyNames":{"type":"string","minLength":1,"maxLength":80,"pattern":"^[!#$%&'*+.^_\u0060|~0-9A-Za-z-]+$"},
      "additionalProperties":{"type":"string","pattern":"^[^\\u0000-\\u0008\\u000a-\\u001f\\u007f]*$"},
      "description":"Static non-secret request headers, including login and refresh. Names must be unique ignoring case. Authentication, transport, and Midwing-managed headers are prohibited. Changing headers requires reconnecting; omitted or empty means no additional headers."
    },
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
          "description":{"type":"string","maxLength":500,"default":"","description":"Permission description; blank or omitted generates a method-specific description."},
          "path":{"type":"string","maxLength":1024,"description":"Allowed path: exact or unique segment placeholders such as /items/{id}; endpoint patterns must not overlap; no queries, traversal, or login/refresh paths."},
          "methods":{"type":"array","items":{"enum":["GET","POST"]},"minItems":1,"maxItems":2,"uniqueItems":true,"default":["GET"],"description":"Explicitly authorized requests for this path. Omitted means GET only. Each method has a separate permission in the desktop app. POST accepts one JSON body."},
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
    {"id":"example-token","name":"Example Token","baseUrl":"https://api.example.com/v1","authKind":"token","header":"Authorization","prefix":"Bearer ","headers":{"X-API-Version":"2026-08-14"},"endpoints":[{"path":"/items","queryParameters":["limit","cursor"]},{"path":"/items/{id}"}]},
    {"id":"example-password","name":"Example Password","baseUrl":"https://api.example.com/v1","authKind":"password","header":"Authorization","prefix":"Bearer ","password":{"loginPath":"/login","emailField":"email","passwordField":"password","tokenField":"session.token","refreshPath":"/refresh"},"endpoints":[{"path":"/items"}]},
    {"id":"example-write","name":"Example Write API","baseUrl":"https://api.example.com/v1","authKind":"token","header":"Authorization","prefix":"Bearer ","endpoints":[{"path":"/items","methods":["GET","POST"],"queryParameters":["limit"]}]}
  ],
  "description":"One JSON object, max 1 MiB, with no credentials. Defaults describe omission behavior; other fields are not inferred. Run services validate FILE|- for authoritative cross-field validation. Validation does not check ID availability. Create the definition, then connect and grant permissions in the desktop app."
}`
