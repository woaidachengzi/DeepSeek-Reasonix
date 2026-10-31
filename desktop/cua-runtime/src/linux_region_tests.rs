// Exercise the actual Linux routing module without a compositor or Linux FFI.
#[path = "../../cua-source/libs/cua-driver/rust/crates/platform-linux/src/wayland/region_route.rs"]
mod region_route;

use region_route::{route_absolute, RegionRect};

fn region(x: f32) -> RegionRect {
    RegionRect {
        offset_x: x,
        offset_y: 0.,
        width: 3840.,
        height: 2160.,
    }
}

#[test]
fn secondary_monitors_select_the_correct_device() {
    let left = [region(0.)];
    let right = [region(3840.)];
    assert_eq!(
        route_absolute([("left", &left[..]), ("right", &right[..])], 6237., 1014.).unwrap(),
        Some("right")
    );
    let both = [left[0], right[0]];
    assert_eq!(
        route_absolute([("wide", &both[..])], 6237., 1014.).unwrap(),
        Some("wide")
    );
    let negative = [region(-3840.)];
    assert_eq!(
        route_absolute([("negative", &negative[..])], -100., 100.).unwrap(),
        Some("negative")
    );
}

#[test]
fn outside_region_is_refused_instead_of_using_a_stale_pointer() {
    let both = [region(0.), region(3840.)];
    let empty: [RegionRect; 0] = [];
    for x in [7680., 8000., f64::NAN, f64::INFINITY] {
        assert!(route_absolute([("empty", &empty[..]), ("wide", &both[..])], x, 100.).is_err());
    }
    assert!(route_absolute([("wide", &both[..])], 100., 2160.).is_err());
}

#[test]
fn fallback_requires_no_announced_regions() {
    let empty: [RegionRect; 0] = [];
    assert_eq!(
        route_absolute([("empty", &empty[..])], 100., 100.).unwrap(),
        None
    );
    assert_eq!(route_absolute::<&str>([], 100., 100.).unwrap(), None);
}
