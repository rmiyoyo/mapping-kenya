function go(lat, lng) {
  location.href = "/at?lat=" + lat.toFixed(5) + "&lng=" + lng.toFixed(5);
}

document.querySelectorAll(".pick svg").forEach(function (svg) {
  svg.addEventListener("click", function (e) {
    var pt = svg.createSVGPoint();
    pt.x = e.clientX;
    pt.y = e.clientY;
    var layer = svg.querySelector(".layer") || svg;
    var p = pt.matrixTransform(layer.getScreenCTM().inverse());
    var scale = parseFloat(svg.dataset.scale);
    go(parseFloat(svg.dataset.lat0) - p.y / scale, parseFloat(svg.dataset.lng0) + p.x / scale);
  });
});

var locate = document.getElementById("locate");
if (locate && navigator.geolocation) {
  locate.hidden = false;
  locate.addEventListener("click", function () {
    var error = document.getElementById("locate-error");
    locate.disabled = true;
    locate.textContent = "Finding you…";
    error.textContent = "";
    navigator.geolocation.getCurrentPosition(
      function (pos) {
        go(pos.coords.latitude, pos.coords.longitude);
      },
      function (err) {
        locate.disabled = false;
        locate.textContent = "Use my location";
        error.textContent = "Couldn't get your location: " + err.message;
      },
      { timeout: 15000 }
    );
  });
}

function openTarget() {
  var target = location.hash && document.getElementById(location.hash.slice(1));
  if (target && target.tagName === "DETAILS") target.open = true;
}
window.addEventListener("hashchange", openTarget);
openTarget();
