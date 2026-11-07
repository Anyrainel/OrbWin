//! Closed observation DATA only. No process, native owner, client reader or action dispatch.
//! Keep this module dormant until the actual passive producer and CLI are reviewed.
use serde::{Deserialize, Serialize};

const LIMIT: usize = 16_384;
const ACTIONS: [&str; 5] = ["prepare-engine", "resume-setup", "start-engine", "upgrade-engine", "review-private-clients"];
const UNKNOWN_REASONS: &[&str] = &["route-not-enabled", "missing-component", "control-cut-unsupported", "tuple-cut-unsupported", "malformed-record", "partial-record-unsupported", "observation-changed", "observation-unknown", "client-inspection-unavailable", "proof-scope"];

#[derive(Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
struct Action { action: String, state: String, reason: String }

#[derive(Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct Inspection {
    schema: u8,
    evidence: String,
    scope: String,
    phase: String,
    reason: String,
    runtime: String,
    image: String,
    connection: String,
    clients: String,
    actions: Vec<Action>,
}

#[derive(Debug, PartialEq)]
pub(crate) struct Unavailable;

fn one(value: &str, choices: &[&str]) -> bool { choices.contains(&value) }

impl Inspection {
    fn valid(&self) -> bool {
        if self.schema != 2 || self.evidence != "bounded-observation" || !one(&self.scope, &["product", "historical-debug-proof"])
            || !one(&self.runtime, &["absent", "uncommitted", "committed", "unknown"])
            || !one(&self.image, &["same", "different", "unknown"])
            || self.connection != "not-checked" || self.clients != "not-checked"
            || self.actions.len() != ACTIONS.len()
            || !self.actions.iter().zip(ACTIONS).all(|(a, name)| a.action == name && a.state == "unsupported" && a.reason == "route-not-enabled")
            || (self.scope == "historical-debug-proof" && (self.phase != "unknown" || self.reason != "proof-scope"))
            || (self.reason == "proof-scope" && self.scope != "historical-debug-proof") { return false; }
        match self.phase.as_str() {
            "fresh" => self.runtime == "absent" && self.image == "unknown" && self.reason == "initial-absence-observed",
            "interrupted" => self.runtime == "uncommitted" && self.image == "same" && one(&self.reason, &["interrupted-same-image", "initialized-uncommitted", "prepared-not-renamed", "renamed-not-committed", "orphan-stage", "missing-control-after-denial"]),
            "committed" => self.runtime == "committed" && self.image == "same" && self.reason == "committed-receipt",
            "unsupported-version" => one(&self.runtime, &["uncommitted", "committed"]) && self.image == "different" && one(&self.reason, &["different-image", "different-source"]),
            "conflict" => self.reason == "namespace-conflict",
            "busy" => self.reason == "owner-busy",
            "unknown" => self.runtime == "unknown" && self.image == "unknown" && one(&self.reason, UNKNOWN_REASONS),
            _ => false,
        }
    }

    /// No role in this contract is enabled. Neither a native observation nor a
    /// future caller-supplied role hint can turn this closed version into a grant.
    pub(crate) fn action_enabled(&self, _action: &str) -> bool { false }
    pub(crate) fn phase(&self) -> &str { &self.phase }
}

/// The future transport must separately own the original child, bounded stdout,
/// stderr, EOF and timeout. Failed/unknown exit cannot supply classification DATA.
pub(crate) fn decode(body: &[u8], successful_exit: bool) -> Result<Inspection, Unavailable> {
    if !successful_exit || body.is_empty() || body.len() > LIMIT { return Err(Unavailable); }
    let result: Inspection = serde_json::from_slice(body).map_err(|_| Unavailable)?;
    if !result.valid() { return Err(Unavailable); }
    let mut canonical = serde_json::to_vec(&result).map_err(|_| Unavailable)?;
    canonical.push(b'\n');
    if body != canonical { return Err(Unavailable); }
    Ok(result)
}

/// DATA bytes for the future single request; caller must then close stdin.
/// There is deliberately no process launcher in this module.
pub(crate) fn request_frame() -> Vec<u8> {
    const REQUEST: &[u8] = br#"{"schema":2,"operation":"inspect"}"#;
    let mut frame = (REQUEST.len() as u32).to_le_bytes().to_vec();
    frame.extend_from_slice(REQUEST);
    frame
}

#[cfg(test)]
mod tests {
    use super::*;
    fn examples() -> Vec<serde_json::Value> { serde_json::from_str(include_str!("testdata/runtime-inspection-wire-examples.json")).unwrap() }
    fn wire(value: &serde_json::Value) -> Vec<u8> {
        // Struct serialization restores the Go-declared order; Value uses sorted keys.
        let r: Inspection = serde_json::from_value(value.clone()).unwrap();
        let mut b = serde_json::to_vec(&r).unwrap(); b.push(b'\n'); b
    }
    #[test] fn actual_go_examples_keep_every_route_disabled() {
        for value in examples() { let bytes=wire(&value); let r=decode(&bytes,true).unwrap();
            assert_eq!(r.phase(),value["phase"].as_str().unwrap());
            for name in ACTIONS.into_iter().chain(["setup","unknown"]) { assert!(!r.action_enabled(name)); }
            assert_eq!(decode(&bytes,false).unwrap_err(),Unavailable);
        }
    }
    #[test] fn frame_is_exact_one_request_without_extra_bytes() {
        let b=request_frame();assert_eq!(u32::from_le_bytes(b[..4].try_into().unwrap()) as usize,b.len()-4);
        assert_eq!(&b[4..],br#"{"schema":2,"operation":"inspect"}"#);
    }
    #[test] fn malformed_aliases_and_output_failures_never_become_absence() {
        let bytes=wire(&examples()[0]);let s=String::from_utf8(bytes.clone()).unwrap();
        let bad=[s.replace("\"schema\":2","\"schema\":2,\"schema\":2"),s.replace("\"schema\":2","\"Schema\":2"),s.replace("\"schema\":2","\"schema\":2,\"extra\":0"),s.replace("\"schema\":2","\"schema\":1"),s.replace("\"unsupported\"","\"available\""),s.replace("\"not-checked\"","\"online\""),s.replace("\"absent\"","\"committed\""),s.replace("\"fresh\"","\"Fresh\""),format!(" {s}"),format!("{s}{s}"),s.trim_end().to_owned(),s.replace('\n',"\r\n")];
        for b in bad { assert!(decode(b.as_bytes(),true).is_err()); }
        for b in [vec![],vec![b'x';LIMIT+1],vec![0xff],{let mut b=vec![0xef,0xbb,0xbf];b.extend(&bytes);b}] { assert!(decode(&b,true).is_err()); }
    }
    #[test] fn action_order_duplicates_proof_scope_and_future_clients_refuse() {
        let base=examples()[0].clone();
        let mut v=base.clone();v["clients"]="ready".into();assert!(decode(&wire(&v),true).is_err());
        let mut v=base.clone();v["actions"].as_array_mut().unwrap().swap(0,1);assert!(decode(&wire(&v),true).is_err());
        let mut v=base.clone();v["actions"][1]=v["actions"][0].clone();assert!(decode(&wire(&v),true).is_err());
        let mut v=base;v["scope"]="historical-debug-proof".into();assert!(decode(&wire(&v),true).is_err());
        let mut v=examples().pop().unwrap();v["scope"]="historical-debug-proof".into();v["reason"]="proof-scope".into();assert!(decode(&wire(&v),true).is_ok());
    }
    #[test] fn schema2_requires_bounded_observation_evidence() {
        let bytes=wire(&examples()[0]); let text=String::from_utf8(bytes).unwrap();
        for evidence in ["atomic-snapshot","readiness","mutation-authority","unique-key",""] {
            assert!(decode(text.replace("bounded-observation",evidence).as_bytes(),true).is_err());
        }
        for bad in [text.replace("\"evidence\":\"bounded-observation\",", ""),text.replace("\"evidence\":", "\"Evidence\":"),text.replace("\"evidence\":\"bounded-observation\"", "\"evidence\":\"bounded-observation\",\"evidence\":\"bounded-observation\""),text.replace("\"schema\":2", "\"schema\":3")] { assert!(decode(bad.as_bytes(),true).is_err()); }
    }

}
