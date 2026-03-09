// Package openai provides utilities to translate OpenAI Chat Completions
// request JSON into OpenAI Responses API request JSON using gjson/sjson.
// It supports tools, multimodal text/image inputs, and Structured Outputs.
// The package handles the conversion of OpenAI API requests into the format
// expected by the OpenAI Responses API, including proper mapping of messages,
// tools, and generation parameters.
package chat_completions

import (
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ConvertOpenAIRequestToCodex converts an OpenAI Chat Completions request JSON
// into an OpenAI Responses API request JSON.
func ConvertOpenAIRequestToCodex(modelName string, inputRawJSON []byte, stream bool) []byte {
	return convertOpenAIRequestToCodexLegacy(modelName, inputRawJSON, stream)
}

// ConvertOpenAIRequestToCodexV2 exposes the optimized translator for tests and benchmarks.
func ConvertOpenAIRequestToCodexV2(modelName string, inputRawJSON []byte, stream bool) []byte {
	return convertOpenAIRequestToCodexV2(modelName, inputRawJSON, stream)
}

// convertOpenAIRequestToCodexLegacy preserves the original gjson/sjson-based translator
// as a battle-tested fallback for malformed or unsupported edge inputs.
func convertOpenAIRequestToCodexLegacy(modelName string, inputRawJSON []byte, stream bool) []byte {
	rawJSON := inputRawJSON
	out := []byte(`{"instructions":""}`)
	out, _ = sjson.SetBytes(out, "stream", stream)

	if v := gjson.GetBytes(rawJSON, "reasoning_effort"); v.Exists() {
		out, _ = sjson.SetBytes(out, "reasoning.effort", v.Value())
	} else {
		out, _ = sjson.SetBytes(out, "reasoning.effort", "medium")
	}
	out, _ = sjson.SetBytes(out, "parallel_tool_calls", true)
	out, _ = sjson.SetBytes(out, "reasoning.summary", "auto")
	out, _ = sjson.SetBytes(out, "include", []string{"reasoning.encrypted_content"})
	out, _ = sjson.SetBytes(out, "model", modelName)

	originalToolNameMap := map[string]string{}
	{
		tools := gjson.GetBytes(rawJSON, "tools")
		if tools.IsArray() && len(tools.Array()) > 0 {
			var names []string
			arr := tools.Array()
			for i := 0; i < len(arr); i++ {
				t := arr[i]
				if t.Get("type").String() == "function" {
					fn := t.Get("function")
					if fn.Exists() {
						if v := fn.Get("name"); v.Exists() {
							names = append(names, v.String())
						}
					}
				}
			}
			if len(names) > 0 {
				originalToolNameMap = buildShortNameMap(names)
			}
		}
	}

	messages := gjson.GetBytes(rawJSON, "messages")
	out, _ = sjson.SetRawBytes(out, "input", []byte(`[]`))
	if messages.IsArray() {
		arr := messages.Array()
		for i := 0; i < len(arr); i++ {
			m := arr[i]
			role := m.Get("role").String()

			switch role {
			case "tool":
				toolCallID := m.Get("tool_call_id").String()
				content := m.Get("content").String()

				funcOutput := []byte(`{}`)
				funcOutput, _ = sjson.SetBytes(funcOutput, "type", "function_call_output")
				funcOutput, _ = sjson.SetBytes(funcOutput, "call_id", toolCallID)
				funcOutput, _ = sjson.SetBytes(funcOutput, "output", content)
				out, _ = sjson.SetRawBytes(out, "input.-1", funcOutput)

			default:
				msg := []byte(`{}`)
				msg, _ = sjson.SetBytes(msg, "type", "message")
				if role == "system" {
					msg, _ = sjson.SetBytes(msg, "role", "developer")
				} else {
					msg, _ = sjson.SetBytes(msg, "role", role)
				}

				msg, _ = sjson.SetRawBytes(msg, "content", []byte(`[]`))

				c := m.Get("content")
				if c.Exists() && c.Type == gjson.String && c.String() != "" {
					partType := "input_text"
					if role == "assistant" {
						partType = "output_text"
					}
					part := []byte(`{}`)
					part, _ = sjson.SetBytes(part, "type", partType)
					part, _ = sjson.SetBytes(part, "text", c.String())
					msg, _ = sjson.SetRawBytes(msg, "content.-1", part)
				} else if c.Exists() && c.IsArray() {
					items := c.Array()
					for j := 0; j < len(items); j++ {
						it := items[j]
						switch it.Get("type").String() {
						case "text":
							partType := "input_text"
							if role == "assistant" {
								partType = "output_text"
							}
							part := []byte(`{}`)
							part, _ = sjson.SetBytes(part, "type", partType)
							part, _ = sjson.SetBytes(part, "text", it.Get("text").String())
							msg, _ = sjson.SetRawBytes(msg, "content.-1", part)
						case "image_url":
							if role == "user" {
								part := []byte(`{}`)
								part, _ = sjson.SetBytes(part, "type", "input_image")
								if u := it.Get("image_url.url"); u.Exists() {
									part, _ = sjson.SetBytes(part, "image_url", u.String())
								}
								msg, _ = sjson.SetRawBytes(msg, "content.-1", part)
							}
						case "file":
							if role == "user" {
								fileData := it.Get("file.file_data").String()
								filename := it.Get("file.filename").String()
								if fileData != "" {
									part := []byte(`{}`)
									part, _ = sjson.SetBytes(part, "type", "input_file")
									part, _ = sjson.SetBytes(part, "file_data", fileData)
									if filename != "" {
										part, _ = sjson.SetBytes(part, "filename", filename)
									}
									msg, _ = sjson.SetRawBytes(msg, "content.-1", part)
								}
							}
						}
					}
				}

				// Don't emit empty assistant messages when only tool_calls
				// are present — Responses API needs function_call items
				// directly, otherwise call_id matching fails (#2132).
				if role != "assistant" || len(gjson.GetBytes(msg, "content").Array()) > 0 {
					out, _ = sjson.SetRawBytes(out, "input.-1", msg)
				}

				if role == "assistant" {
					toolCalls := m.Get("tool_calls")
					if toolCalls.Exists() && toolCalls.IsArray() {
						toolCallsArr := toolCalls.Array()
						for j := 0; j < len(toolCallsArr); j++ {
							tc := toolCallsArr[j]
							if tc.Get("type").String() == "function" {
								funcCall := []byte(`{}`)
								funcCall, _ = sjson.SetBytes(funcCall, "type", "function_call")
								funcCall, _ = sjson.SetBytes(funcCall, "call_id", tc.Get("id").String())
								{
									name := tc.Get("function.name").String()
									if short, ok := originalToolNameMap[name]; ok {
										name = short
									} else {
										name = shortenNameIfNeeded(name)
									}
									funcCall, _ = sjson.SetBytes(funcCall, "name", name)
								}
								funcCall, _ = sjson.SetBytes(funcCall, "arguments", tc.Get("function.arguments").String())
								out, _ = sjson.SetRawBytes(out, "input.-1", funcCall)
							}
						}
					}
				}
			}
		}
	}

	rf := gjson.GetBytes(rawJSON, "response_format")
	text := gjson.GetBytes(rawJSON, "text")
	if rf.Exists() {
		if !gjson.GetBytes(out, "text").Exists() {
			out, _ = sjson.SetRawBytes(out, "text", []byte(`{}`))
		}
		switch rf.Get("type").String() {
		case "text":
			out, _ = sjson.SetBytes(out, "text.format.type", "text")
		case "json_schema":
			js := rf.Get("json_schema")
			if js.Exists() {
				out, _ = sjson.SetBytes(out, "text.format.type", "json_schema")
				if v := js.Get("name"); v.Exists() {
					out, _ = sjson.SetBytes(out, "text.format.name", v.Value())
				}
				if v := js.Get("strict"); v.Exists() {
					out, _ = sjson.SetBytes(out, "text.format.strict", v.Value())
				}
				if v := js.Get("schema"); v.Exists() {
					out, _ = sjson.SetRawBytes(out, "text.format.schema", []byte(v.Raw))
				}
			}
		}
		if text.Exists() {
			if v := text.Get("verbosity"); v.Exists() {
				out, _ = sjson.SetBytes(out, "text.verbosity", v.Value())
			}
		}
	} else if text.Exists() {
		if v := text.Get("verbosity"); v.Exists() {
			if !gjson.GetBytes(out, "text").Exists() {
				out, _ = sjson.SetRawBytes(out, "text", []byte(`{}`))
			}
			out, _ = sjson.SetBytes(out, "text.verbosity", v.Value())
		}
	}

	tools := gjson.GetBytes(rawJSON, "tools")
	if tools.IsArray() && len(tools.Array()) > 0 {
		out, _ = sjson.SetRawBytes(out, "tools", []byte(`[]`))
		arr := tools.Array()
		for i := 0; i < len(arr); i++ {
			t := arr[i]
			toolType := t.Get("type").String()
			if toolType != "" && toolType != "function" && t.IsObject() {
				out, _ = sjson.SetRawBytes(out, "tools.-1", []byte(t.Raw))
				continue
			}
			if toolType == "function" {
				item := []byte(`{}`)
				item, _ = sjson.SetBytes(item, "type", "function")
				fn := t.Get("function")
				if fn.Exists() {
					if v := fn.Get("name"); v.Exists() {
						name := v.String()
						if short, ok := originalToolNameMap[name]; ok {
							name = short
						} else {
							name = shortenNameIfNeeded(name)
						}
						item, _ = sjson.SetBytes(item, "name", name)
					}
					if v := fn.Get("description"); v.Exists() {
						item, _ = sjson.SetBytes(item, "description", v.Value())
					}
					if v := fn.Get("parameters"); v.Exists() {
						item, _ = sjson.SetRawBytes(item, "parameters", []byte(v.Raw))
					}
					if v := fn.Get("strict"); v.Exists() {
						item, _ = sjson.SetBytes(item, "strict", v.Value())
					}
				}
				out, _ = sjson.SetRawBytes(out, "tools.-1", item)
			}
		}
	}

	if tc := gjson.GetBytes(rawJSON, "tool_choice"); tc.Exists() {
		switch {
		case tc.Type == gjson.String:
			out, _ = sjson.SetBytes(out, "tool_choice", tc.String())
		case tc.IsObject():
			tcType := tc.Get("type").String()
			if tcType == "function" {
				name := tc.Get("function.name").String()
				if name != "" {
					if short, ok := originalToolNameMap[name]; ok {
						name = short
					} else {
						name = shortenNameIfNeeded(name)
					}
				}
				choice := []byte(`{}`)
				choice, _ = sjson.SetBytes(choice, "type", "function")
				if name != "" {
					choice, _ = sjson.SetBytes(choice, "name", name)
				}
				out, _ = sjson.SetRawBytes(out, "tool_choice", choice)
			} else if tcType != "" {
				out, _ = sjson.SetRawBytes(out, "tool_choice", []byte(tc.Raw))
			}
		}
	}

	out, _ = sjson.SetBytes(out, "store", false)
	return out
}

func shortenNameIfNeeded(name string) string {
	const limit = 64
	if len(name) <= limit {
		return name
	}
	if strings.HasPrefix(name, "mcp__") {
		idx := strings.LastIndex(name, "__")
		if idx > 0 {
			candidate := "mcp__" + name[idx+2:]
			if len(candidate) > limit {
				return candidate[:limit]
			}
			return candidate
		}
	}
	return name[:limit]
}

func buildShortNameMap(names []string) map[string]string {
	const limit = 64
	used := map[string]struct{}{}
	m := map[string]string{}

	baseCandidate := func(n string) string {
		if len(n) <= limit {
			return n
		}
		if strings.HasPrefix(n, "mcp__") {
			idx := strings.LastIndex(n, "__")
			if idx > 0 {
				cand := "mcp__" + n[idx+2:]
				if len(cand) > limit {
					cand = cand[:limit]
				}
				return cand
			}
		}
		return n[:limit]
	}

	makeUnique := func(cand string) string {
		if _, ok := used[cand]; !ok {
			return cand
		}
		base := cand
		for i := 1; ; i++ {
			suffix := "_" + strconv.Itoa(i)
			allowed := limit - len(suffix)
			if allowed < 0 {
				allowed = 0
			}
			tmp := base
			if len(tmp) > allowed {
				tmp = tmp[:allowed]
			}
			tmp = tmp + suffix
			if _, ok := used[tmp]; !ok {
				return tmp
			}
		}
	}

	for _, n := range names {
		cand := baseCandidate(n)
		uniq := makeUnique(cand)
		used[uniq] = struct{}{}
		m[n] = uniq
	}
	return m
}
