//! A detached SDK service keeps its own cache lease. Accept that handoff only
//! for the independently pinned bundle and an actual native process generation.
//! The SDK client separately authenticates the named-pipe peer before requests.
use serde::Deserialize;

#[derive(Deserialize)]
#[serde(deny_unknown_fields, rename_all = "camelCase")]
struct Identity {
    schema: u8,
    pid: u32,
    creation_time: String,
    session: u32,
    image_sha256: String,
    bundle_sha256: String,
    lease_held: bool,
}
const REFUSAL: &str = "Local service executable ownership could not be confirmed. Its data is retained; inspect the service before retrying a change.";

impl Identity {
    fn validate(&self, image: &str, bundle: &str) -> Result<u64, String> {
        let created = self
            .creation_time
            .parse::<u64>()
            .map_err(|_| REFUSAL.to_string())?;
        if self.schema != 1
            || self.pid == 0
            || self.session == 0
            || !self.lease_held
            || created == 0
            || created.to_string() != self.creation_time
            || self.image_sha256 != image
            || self.bundle_sha256 != bundle
        {
            return Err(REFUSAL.into());
        }
        Ok(created)
    }
}

pub(crate) fn verify_snapshot(
    binary: &crate::runtime_cache::CachedExecutable,
    response: &serde_json::Value,
    child: Option<&std::process::Child>,
) -> Result<(), String> {
    let raw = response
        .get("state")
        .and_then(|state| state.get("selfIdentity"))
        .ok_or(REFUSAL)?;
    let identity: Identity =
        serde_json::from_value(raw.clone()).map_err(|_| REFUSAL.to_string())?;
    let created = identity.validate(
        &crate::runtime::component_binary_pin("orbwin-session.exe")?,
        &crate::runtime::component_binary_pin("lan/assets.json")?,
    )?;
    binary
        .verify_process(identity.pid, created, identity.session, child)
        .map_err(|_| REFUSAL.to_string())
}

#[cfg(test)]
mod tests {
    use super::*;
    fn valid() -> serde_json::Value {
        serde_json::json!({"schema":1,"pid":42,"creationTime":"134000000000000000","session":1,
            "imageSha256":"a".repeat(64),"bundleSha256":"b".repeat(64),"leaseHeld":true})
    }
    #[test]
    fn handoff_requires_exact_build_bundle_and_positive_generation() {
        let validate = |value| {
            serde_json::from_value::<Identity>(value)
                .ok()
                .and_then(|identity| identity.validate(&"a".repeat(64), &"b".repeat(64)).ok())
        };
        assert!(validate(valid()).is_some());
        for (field, value) in [
            ("schema", 2.into()),
            ("pid", 0.into()),
            ("session", 0.into()),
            ("leaseHeld", false.into()),
            ("imageSha256", "c".repeat(64).into()),
            ("bundleSha256", "c".repeat(64).into()),
            ("creationTime", "0".into()),
            ("creationTime", "0134000000000000000".into()),
            ("creationTime", "+134000000000000000".into()),
            ("creationTime", "18446744073709551616".into()),
            ("unrecognized", true.into()),
        ] {
            let mut value_under_test = valid();
            value_under_test[field] = value;
            assert!(validate(value_under_test).is_none(), "{field}");
        }
        for field in valid().as_object().unwrap().keys() {
            let mut missing = valid();
            missing.as_object_mut().unwrap().remove(field);
            assert!(validate(missing).is_none(), "missing {field}");
        }
    }
}
