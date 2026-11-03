// A photo's small copy, for lists, grids and strips (handlers/photo_thumbs.go,
// PhotoThumbURL does the same for templates). Full-size views keep the original.
window.bmThumb = u => /^\/uploads\/(plants|smoke)\//.test(u || "") ? "/thumbs" + u.slice("/uploads".length) : u;
