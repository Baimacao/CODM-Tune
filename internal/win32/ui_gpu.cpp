// ui_gpu.cpp - Direct2D + Direct3D 11 backend for the CODM Tune UI.
//
// The GDI+ path in ui_shim.cpp rasterises every frame on the CPU: a 1100x700
// layer costs ~14 ms of software work and then reaches the window through a
// DIB section and BitBlt. This file draws the same primitives through D2D1
// onto a DXGI swap chain, so rasterisation and present both happen on the GPU.
//
// The ABI stays integer-only for the same reason as the GDI+ shim: Go cannot
// pass floats through syscall.SyscallN. Colours arrive as 0xAARRGGBB.
//
// Both backends are compiled into codmtune_ui.dll. Nothing here is called
// unless gtg_init() succeeds, so a machine without D3D11 keeps the GDI+ path.

#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <d2d1_1.h>
#include <d3d11_1.h>
#include <dxgi1_2.h>
#include <dwrite.h>
#include <wincodec.h>
#include <wrl/client.h>
#include <math.h>
#include <string>
#include <vector>

using namespace Microsoft::WRL;

namespace {

struct GpuState {
    bool ok = false;
    ComPtr<ID3D11Device> d3d;
    ComPtr<IDXGIDevice> dxgiDevice;
    ComPtr<ID2D1Factory1> factory;
    ComPtr<ID2D1Device> device;
    ComPtr<IDWriteFactory> dwrite;
    ComPtr<IWICImagingFactory> wic;
    ComPtr<IDWriteInlineObject> ellipsis;
    std::wstring familyName;
    std::wstring iconName;
};

GpuState g;

// 0xAARRGGBB to D2D1_COLOR_F.
//
// D2D1_COLOR_F is NOT premultiplied: the brush applies alpha itself. Scaling
// RGB by alpha here as well darkened every translucent colour, which showed up
// as purple where the theme asks for a red fill.
inline D2D1_COLOR_F toColor(unsigned argb) {
    D2D1_COLOR_F c;
    c.a = ((argb >> 24) & 0xFF) / 255.0f;
    c.r = ((argb >> 16) & 0xFF) / 255.0f;
    c.g = ((argb >> 8) & 0xFF) / 255.0f;
    c.b = (argb & 0xFF) / 255.0f;
    return c;
}

struct GpuSurface {
    HWND hwnd = nullptr;
    UINT w = 0, h = 0;
    ComPtr<IDXGISwapChain1> swap;
    ComPtr<ID2D1DeviceContext> ctx;
    ComPtr<ID2D1Bitmap1> target;
};

struct GpuCanvas {
    GpuSurface* surf = nullptr;
    float scale = 1.0f;
    int offX = 0, offY = 0;
    int clipN = 0;
    D2D1_RECT_F clipRect[32];
};

struct BrushRec {
    unsigned argb;
    ComPtr<ID2D1SolidColorBrush> brush;
};
std::vector<BrushRec> g_brushes;

struct TextRec {
    int size;
    int bold;
    ComPtr<IDWriteTextFormat> fmt;
};
std::vector<TextRec> g_texts;

struct ImageRec {
    std::wstring path;
    ComPtr<ID2D1Bitmap> bitmap;
    int w = 0, h = 0;
};
std::vector<ImageRec> g_images;

ID2D1SolidColorBrush* getBrush(GpuCanvas* c, unsigned argb) {
    if (!c || !c->surf || !c->surf->ctx) return nullptr;
    for (auto& r : g_brushes) {
        if (r.argb == argb) {
            r.brush->SetColor(toColor(argb));
            return r.brush.Get();
        }
    }
    ComPtr<ID2D1SolidColorBrush> b;
    if (FAILED(c->surf->ctx->CreateSolidColorBrush(toColor(argb), &b))) return nullptr;
    BrushRec rec;
    rec.argb = argb;
    rec.brush = b;
    g_brushes.push_back(rec);
    return g_brushes.back().brush.Get();
}

IDWriteTextFormat* getFormat(int size, int bold) {
    for (auto& r : g_texts)
        if (r.size == size && r.bold == bold) return r.fmt.Get();
    if (!g.dwrite) return nullptr;
    ComPtr<IDWriteTextFormat> fmt;
    if (FAILED(g.dwrite->CreateTextFormat(g.familyName.c_str(), nullptr,
            bold ? DWRITE_FONT_WEIGHT_BOLD : DWRITE_FONT_WEIGHT_NORMAL,
            DWRITE_FONT_STYLE_NORMAL, DWRITE_FONT_STRETCH_NORMAL,
            (FLOAT)size, L"", &fmt)))
        return nullptr;
    // The GDI+ shim centres every string vertically in its rect; match that,
    // and trim with an ellipsis because most labels are single-line.
    fmt->SetParagraphAlignment(DWRITE_PARAGRAPH_ALIGNMENT_CENTER);
    fmt->SetWordWrapping(DWRITE_WORD_WRAPPING_NO_WRAP);
    DWRITE_TRIMMING tr;
    tr.granularity = DWRITE_TRIMMING_GRANULARITY_CHARACTER;
    tr.delimiter = 0;
    tr.delimiterCount = 0;
    fmt->SetTrimming(&tr, g.ellipsis.Get());
    TextRec rec;
    rec.size = size;
    rec.bold = bold;
    rec.fmt = fmt;
    g_texts.push_back(rec);
    return g_texts.back().fmt.Get();
}

ID2D1Bitmap* getImage(GpuCanvas* c, const WCHAR* path, int* outW, int* outH) {
    if (outW) *outW = 0;
    if (outH) *outH = 0;
    if (!c || !c->surf || !c->surf->ctx || !path) return nullptr;
    for (auto& r : g_images)
        if (r.path == path) {
            if (outW) *outW = r.w;
            if (outH) *outH = r.h;
            return r.bitmap.Get();
        }
    if (!g.wic) return nullptr;
    ComPtr<IWICBitmapDecoder> dec;
    if (FAILED(g.wic->CreateDecoderFromFilename(path, nullptr, GENERIC_READ,
            WICDecodeMetadataCacheOnLoad, &dec)))
        return nullptr;
    ComPtr<IWICBitmapFrameDecode> frame;
    if (FAILED(dec->GetFrame(0, &frame))) return nullptr;
    ComPtr<IWICFormatConverter> conv;
    if (FAILED(g.wic->CreateFormatConverter(&conv))) return nullptr;
    if (FAILED(conv->Initialize(frame.Get(), GUID_WICPixelFormat32bppPBGRA,
            WICBitmapDitherTypeNone, nullptr, 0.0f, WICBitmapPaletteTypeCustom)))
        return nullptr;
    ComPtr<ID2D1Bitmap> bmp;
    if (FAILED(c->surf->ctx->CreateBitmapFromWicBitmap(conv.Get(), nullptr, &bmp)))
        return nullptr;
    UINT iw = 0, ih = 0;
    conv->GetSize(&iw, &ih);
    if (g_images.size() >= 8) g_images.erase(g_images.begin());
    ImageRec rec;
    rec.path = path;
    rec.bitmap = bmp;
    rec.w = (int)iw;
    rec.h = (int)ih;
    g_images.push_back(rec);
    if (outW) *outW = rec.w;
    if (outH) *outH = rec.h;
    return g_images.back().bitmap.Get();
}

ComPtr<ID2D1PathGeometry> roundPath(float x, float y, float w, float h, float rad) {
    if (rad > w / 2) rad = w / 2;
    if (rad > h / 2) rad = h / 2;
    ComPtr<ID2D1PathGeometry> geo;
    if (!g.factory || FAILED(g.factory->CreatePathGeometry(&geo))) return nullptr;
    ComPtr<ID2D1GeometrySink> sink;
    if (FAILED(geo->Open(&sink))) return nullptr;
    sink->BeginFigure(D2D1::Point2F(x + rad, y), D2D1_FIGURE_BEGIN_FILLED);
    if (rad < 0.5f) {
        sink->AddLine(D2D1::Point2F(x + w, y));
        sink->AddLine(D2D1::Point2F(x + w, y + h));
        sink->AddLine(D2D1::Point2F(x, y + h));
        sink->AddLine(D2D1::Point2F(x, y));
    } else {
        sink->AddLine(D2D1::Point2F(x + w - rad, y));
        D2D1_ARC_SEGMENT a = {};
        a.size = D2D1::SizeF(rad, rad);
        a.rotationAngle = 0;
        a.sweepDirection = D2D1_SWEEP_DIRECTION_CLOCKWISE;
        a.arcSize = D2D1_ARC_SIZE_SMALL;
        a.point = D2D1::Point2F(x + w, y + rad);
        sink->AddArc(a);
        sink->AddLine(D2D1::Point2F(x + w, y + h - rad));
        a.point = D2D1::Point2F(x + w - rad, y + h);
        sink->AddArc(a);
        sink->AddLine(D2D1::Point2F(x + rad, y + h));
        a.point = D2D1::Point2F(x, y + h - rad);
        sink->AddArc(a);
        sink->AddLine(D2D1::Point2F(x, y + rad));
        a.point = D2D1::Point2F(x + rad, y);
        sink->AddArc(a);
    }
    sink->EndFigure(D2D1_FIGURE_END_CLOSED);
    sink->Close();
    return geo;
}

bool bindTarget(GpuSurface* s) {
    s->target.Reset();
    ComPtr<IDXGISurface> back;
    if (FAILED(s->swap->GetBuffer(0, IID_PPV_ARGS(&back)))) return false;
    D2D1_BITMAP_PROPERTIES1 props = D2D1::BitmapProperties1(
        D2D1_BITMAP_OPTIONS_TARGET | D2D1_BITMAP_OPTIONS_CANNOT_DRAW,
        D2D1::PixelFormat(DXGI_FORMAT_B8G8R8A8_UNORM, D2D1_ALPHA_MODE_IGNORE), 96.0f, 96.0f);
    if (FAILED(s->ctx->CreateBitmapFromDxgiSurface(back.Get(), &props, &s->target))) return false;
    s->ctx->SetTarget(s->target.Get());
    return true;
}

// gtg_shadow: a drop shadow drawn from a cached 9-slice bitmap.
//
// The first version stacked a dozen offset rounded rects with falling alpha.
// That measured at 6.03 ms/frame against 5.90 ms for the software path -- each
// rect meant a fresh path geometry plus a brush, which is far more work than
// the thing it was trying to avoid. A shadow does not change shape, only size,
// so it is rasterised once into a small bitmap and then nine-slice stretched.
//
// The bitmap is generated on the CPU: for each pixel, the signed distance to
// the rounded-rect edge decides the alpha, which gives a real soft falloff
// without a blur pass.
struct ShadowCache {
    ComPtr<ID2D1Bitmap> bmp;
    int tile = 0;   // half-size of the cached tile
    int inset = 0;  // corner region that must not be stretched
    int blur = 0;
};
ShadowCache g_shadow;

// distanceToRoundRect returns the signed distance from (px,py) to the edge of a
// rounded rect of size (w,h) with corner radius r; negative means inside.
float distanceToRoundRect(float px, float py, float w, float h, float r) {
    float cx = w / 2.0f, cy = h / 2.0f;
    float qx = fabsf(px - cx) - (cx - r);
    float qy = fabsf(py - cy) - (cy - r);
    float ax = qx > 0 ? qx : 0.0f;
    float ay = qy > 0 ? qy : 0.0f;
    float outside = sqrtf(ax * ax + ay * ay);
    float inside = fminf(fmaxf(qx, qy), 0.0f);
    return outside + inside - r;
}

bool buildShadow(GpuCanvas* c, int blur, unsigned argb) {
    if (!c || !c->surf || !c->surf->ctx) return false;
    int inset = blur * 2;
    int tile = inset + 8;
    int side = tile * 2;
    if (side <= 0 || side > 512) return false;

    // BGRA premultiplied, matching DXGI_FORMAT_B8G8R8A8_UNORM + PREMULTIPLIED.
    std::vector<unsigned> px((size_t)side * side);
    float peak = ((argb >> 24) & 0xFF) / 255.0f;
    float sr = ((argb >> 16) & 0xFF) / 255.0f;
    float sg = ((argb >> 8) & 0xFF) / 255.0f;
    float sb = (argb & 0xFF) / 255.0f;
    float r = 12.0f;
    for (int y = 0; y < side; y++) {
        for (int x = 0; x < side; x++) {
            float d = distanceToRoundRect(x + 0.5f, y + 0.5f, (float)side, (float)side, r);
            // Inside the shape the shadow is at full strength; outside it falls
            // off over `blur` pixels with a quadratic curve.
            float a = 0.0f;
            if (d <= 0) {
                a = peak;
            } else if (d < (float)blur) {
                float t = 1.0f - d / (float)blur;
                a = peak * t * t;
            }
            unsigned alpha = (unsigned)(a * 255.0f + 0.5f);
            if (alpha == 0) {
                px[(size_t)y * side + x] = 0;
                continue;
            }
            unsigned ba = (unsigned)(sb * alpha + 0.5f);
            unsigned ga = (unsigned)(sg * alpha + 0.5f);
            unsigned ra = (unsigned)(sr * alpha + 0.5f);
            px[(size_t)y * side + x] = (alpha << 24) | (ra << 16) | (ga << 8) | ba;
        }
    }

    D2D1_BITMAP_PROPERTIES props = D2D1::BitmapProperties(
        D2D1::PixelFormat(DXGI_FORMAT_B8G8R8A8_UNORM, D2D1_ALPHA_MODE_PREMULTIPLIED));
    ComPtr<ID2D1Bitmap> bmp;
    if (FAILED(c->surf->ctx->CreateBitmap(
            D2D1::SizeU(side, side), px.data(), (UINT32)(side * 4), props, &bmp)))
        return false;
    g_shadow.bmp = bmp;
    g_shadow.tile = tile;
    g_shadow.inset = inset;
    g_shadow.blur = blur;
    return true;
}

// The 9-slice version measured 3.60 ms/frame: nine DrawBitmap calls per card,
// 108 per frame for twelve cards. Card sizes are few and fixed, so the whole
// shadow is baked once per (w,h) and blitted with a single call.
struct ShadowBaked {
    int w, h, blur;
    ComPtr<ID2D1Bitmap> bmp;
};
std::vector<ShadowBaked> g_shadowBaked;

ID2D1Bitmap* bakedShadow(GpuCanvas* c, int w, int h, int blur, unsigned argb) {
    for (auto& e : g_shadowBaked)
        if (e.w == w && e.h == h && e.blur == blur) return e.bmp.Get();
    if (!c || !c->surf || !c->surf->ctx) return nullptr;

    int inset = blur * 2;
    int sw = w + inset * 2;
    int sh = h + inset * 2;
    if (sw <= 0 || sh <= 0 || sw > 2048 || sh > 2048) return nullptr;

    std::vector<unsigned> px((size_t)sw * sh);
    float peak = ((argb >> 24) & 0xFF) / 255.0f;
    float sr = ((argb >> 16) & 0xFF) / 255.0f;
    float sg = ((argb >> 8) & 0xFF) / 255.0f;
    float sb = (argb & 0xFF) / 255.0f;
    float rad = 12.0f;
    for (int y = 0; y < sh; y++) {
        for (int x = 0; x < sw; x++) {
            float d = distanceToRoundRect((float)x + 0.5f, (float)y + 0.5f, (float)sw,
                                          (float)sh, rad);
            float a = 0.0f;
            if (d <= 0) {
                a = peak;
            } else if (d < (float)blur) {
                float t = 1.0f - d / (float)blur;
                a = peak * t * t;
            }
            unsigned alpha = (unsigned)(a * 255.0f + 0.5f);
            if (alpha == 0) {
                px[(size_t)y * sw + x] = 0;
                continue;
            }
            unsigned ba = (unsigned)(sb * alpha + 0.5f);
            unsigned ga = (unsigned)(sg * alpha + 0.5f);
            unsigned ra = (unsigned)(sr * alpha + 0.5f);
            px[(size_t)y * sw + x] = (alpha << 24) | (ra << 16) | (ga << 8) | ba;
        }
    }
    D2D1_BITMAP_PROPERTIES props = D2D1::BitmapProperties(
        D2D1::PixelFormat(DXGI_FORMAT_B8G8R8A8_UNORM, D2D1_ALPHA_MODE_PREMULTIPLIED));
    ComPtr<ID2D1Bitmap> bmp;
    if (FAILED(c->surf->ctx->CreateBitmap(D2D1::SizeU((UINT32)sw, (UINT32)sh), px.data(),
                                          (UINT32)(sw * 4), props, &bmp)))
        return nullptr;
    // Card sizes are a handful; cap the cache so a resized window cannot grow
    // it without bound.
    if (g_shadowBaked.size() >= 8) g_shadowBaked.erase(g_shadowBaked.begin());
    ShadowBaked rec;
    rec.w = w;
    rec.h = h;
    rec.blur = blur;
    rec.bmp = bmp;
    g_shadowBaked.push_back(rec);
    return g_shadowBaked.back().bmp.Get();
}

// drawNineSlice stretches the cached tile to r while keeping the corners 1:1.
void drawNineSlice(GpuCanvas* c, const D2D1_RECT_F& r, float opacity) {
    ID2D1Bitmap* b = g_shadow.bmp.Get();
    if (!b) return;
    float side = (float)(g_shadow.tile * 2);
    float ins = (float)g_shadow.inset;
    float cw = r.right - r.left;
    float ch = r.bottom - r.top;
    if (cw <= 0 || ch <= 0) return;

    // Horizontal and vertical bands: [0..ins], [ins..side-ins], [side-ins..side]
    float sx[4] = {0, ins, side - ins, side};
    float sy[4] = {0, ins, side - ins, side};
    float dx[4] = {r.left, r.left + ins, r.right - ins, r.right};
    float dy[4] = {r.top, r.top + ins, r.bottom - ins, r.bottom};

    for (int i = 0; i < 3; i++) {
        for (int j = 0; j < 3; j++) {
            D2D1_RECT_F src = D2D1::RectF(sx[i], sy[j], sx[i + 1], sy[j + 1]);
            D2D1_RECT_F dst = D2D1::RectF(dx[i], dy[j], dx[i + 1], dy[j + 1]);
            if (dst.right - dst.left <= 0 || dst.bottom - dst.top <= 0) continue;
            c->surf->ctx->DrawBitmap(b, &dst, opacity, D2D1_BITMAP_INTERPOLATION_MODE_LINEAR,
                                     &src);
        }
    }
}


// Gradient brushes are cached by colour pair. Creating a stop collection and a
// brush per card per frame was the same mistake the shadow made: the UI draws
// the same two or three gradients hundreds of times, and each creation is a
// device allocation. The brush geometry is normalised to a unit square and the
// transform is set per call instead, so one brush serves every size.
struct GradientRec {
    unsigned c1, c2;
    bool vertical;
    ComPtr<ID2D1LinearGradientBrush> brush;
};
std::vector<GradientRec> g_gradients;

ID2D1LinearGradientBrush* getGradient(GpuCanvas* c, unsigned c1, unsigned c2, bool vertical) {
    if (!c || !c->surf || !c->surf->ctx) return nullptr;
    for (auto& e : g_gradients)
        if (e.c1 == c1 && e.c2 == c2 && e.vertical == vertical) return e.brush.Get();

    ComPtr<ID2D1GradientStopCollection> stops;
    D2D1_GRADIENT_STOP gs[2];
    gs[0].position = 0.0f;
    gs[0].color = toColor(c1);
    gs[1].position = 1.0f;
    gs[1].color = toColor(c2);
    if (FAILED(c->surf->ctx->CreateGradientStopCollection(gs, 2, &stops))) return nullptr;
    // Unit-space endpoints; the caller scales the brush to the rect.
    D2D1_POINT_2F a = D2D1::Point2F(0, 0);
    D2D1_POINT_2F b = vertical ? D2D1::Point2F(0, 1) : D2D1::Point2F(1, 0);
    ComPtr<ID2D1LinearGradientBrush> br;
    if (FAILED(c->surf->ctx->CreateLinearGradientBrush(
            D2D1::LinearGradientBrushProperties(a, b), stops.Get(), &br)))
        return nullptr;
    if (g_gradients.size() >= 16) g_gradients.erase(g_gradients.begin());
    GradientRec rec;
    rec.c1 = c1;
    rec.c2 = c2;
    rec.vertical = vertical;
    rec.brush = br;
    g_gradients.push_back(rec);
    return g_gradients.back().brush.Get();
}

} // namespace

extern "C" {

__declspec(dllexport) int gtg_init(void) {
    if (g.ok) return 0;
    // WIC and the DXGI factory are COM objects; CoCreateInstance fails with
    // CO_E_NOTINITIALIZED unless this thread has an apartment. RPC_E_CHANGED_MODE
    // means the thread already lives in an MTA, which is fine for our use.
    HRESULT com = CoInitializeEx(nullptr, COINIT_APARTMENTTHREADED);
    if (FAILED(com) && com != RPC_E_CHANGED_MODE) return 7;
    UINT flags = D3D11_CREATE_DEVICE_BGRA_SUPPORT;
    D3D_FEATURE_LEVEL levels[] = {D3D_FEATURE_LEVEL_11_1, D3D_FEATURE_LEVEL_11_0,
                                  D3D_FEATURE_LEVEL_10_1, D3D_FEATURE_LEVEL_10_0};
    D3D_FEATURE_LEVEL got;
    HRESULT hr = D3D11CreateDevice(nullptr, D3D_DRIVER_TYPE_HARDWARE, nullptr, flags,
                                   levels, 4, D3D11_SDK_VERSION, &g.d3d, &got, nullptr);
    if (FAILED(hr))
        hr = D3D11CreateDevice(nullptr, D3D_DRIVER_TYPE_WARP, nullptr, flags, levels, 4,
                               D3D11_SDK_VERSION, &g.d3d, &got, nullptr);
    if (FAILED(hr)) return 1;
    if (FAILED(g.d3d.As(&g.dxgiDevice))) return 2;
    if (FAILED(D2D1CreateFactory(D2D1_FACTORY_TYPE_SINGLE_THREADED, __uuidof(ID2D1Factory1),
                                 nullptr, (void**)&g.factory)))
        return 3;
    if (FAILED(g.factory->CreateDevice(g.dxgiDevice.Get(), &g.device))) return 4;
    if (FAILED(DWriteCreateFactory(DWRITE_FACTORY_TYPE_SHARED, __uuidof(IDWriteFactory),
                                   (IUnknown**)&g.dwrite)))
        return 5;
    if (FAILED(CoCreateInstance(CLSID_WICImagingFactory, nullptr, CLSCTX_INPROC_SERVER,
                                IID_PPV_ARGS(&g.wic))))
        return 6;

    ComPtr<IDWriteFontCollection> coll;
    if (SUCCEEDED(g.dwrite->GetSystemFontCollection(&coll))) {
        const WCHAR* names[4] = {L"Microsoft YaHei UI", L"Microsoft YaHei", L"Segoe UI", L"Arial"};
        for (int i = 0; i < 4 && g.familyName.empty(); i++) {
            UINT32 idx = 0;
            BOOL exists = FALSE;
            if (SUCCEEDED(coll->FindFamilyName(names[i], &idx, &exists)) && exists)
                g.familyName = names[i];
        }
        const WCHAR* icons[2] = {L"Segoe MDL2 Assets", L"Segoe Fluent Icons"};
        for (int i = 0; i < 2 && g.iconName.empty(); i++) {
            UINT32 idx = 0;
            BOOL exists = FALSE;
            if (SUCCEEDED(coll->FindFamilyName(icons[i], &idx, &exists)) && exists)
                g.iconName = icons[i];
        }
    }
    if (g.familyName.empty()) g.familyName = L"Segoe UI";

    ComPtr<IDWriteTextFormat> tmp;
    if (SUCCEEDED(g.dwrite->CreateTextFormat(g.familyName.c_str(), nullptr,
            DWRITE_FONT_WEIGHT_NORMAL, DWRITE_FONT_STYLE_NORMAL, DWRITE_FONT_STRETCH_NORMAL,
            12.0f, L"", &tmp)))
        g.dwrite->CreateEllipsisTrimmingSign(tmp.Get(), &g.ellipsis);

    g.ok = true;
    return 0;
}

__declspec(dllexport) void gtg_shutdown(void) {
    // Order matters: every bitmap and brush holds a reference to the device
    // context, so they must all go before the device itself.
    g_shadow.bmp.Reset();
    g_shadowBaked.clear();
    g_gradients.clear();
    g_brushes.clear();
    g_texts.clear();
    g_images.clear();
    g.ellipsis.Reset();
    g.wic.Reset();
    g.dwrite.Reset();
    g.device.Reset();
    g.factory.Reset();
    g.dxgiDevice.Reset();
    g.d3d.Reset();
    g.ok = false;
}

__declspec(dllexport) int gtg_version(void) { return 1; }

__declspec(dllexport) void* gtg_surface_new(void* hwnd, int w, int h) {
    if (!g.ok || !hwnd || w <= 0 || h <= 0) return nullptr;
    GpuSurface* s = new GpuSurface();
    s->hwnd = (HWND)hwnd;
    s->w = (UINT)w;
    s->h = (UINT)h;

    ComPtr<IDXGIFactory2> factory;
    ComPtr<IDXGIAdapter> adapter;
    if (FAILED(g.dxgiDevice->GetAdapter(&adapter)) ||
        FAILED(adapter->GetParent(IID_PPV_ARGS(&factory)))) {
        delete s;
        return nullptr;
    }
    DXGI_SWAP_CHAIN_DESC1 desc = {};
    desc.Width = s->w;
    desc.Height = s->h;
    desc.Format = DXGI_FORMAT_B8G8R8A8_UNORM;
    desc.SampleDesc.Count = 1;
    desc.BufferUsage = DXGI_USAGE_RENDER_TARGET_OUTPUT;
    desc.BufferCount = 2;
    desc.SwapEffect = DXGI_SWAP_EFFECT_FLIP_SEQUENTIAL;
    desc.AlphaMode = DXGI_ALPHA_MODE_IGNORE;
    if (FAILED(factory->CreateSwapChainForHwnd(g.d3d.Get(), s->hwnd, &desc, nullptr, nullptr,
                                               &s->swap))) {
        desc.SwapEffect = DXGI_SWAP_EFFECT_DISCARD;
        desc.BufferCount = 1;
        if (FAILED(factory->CreateSwapChainForHwnd(g.d3d.Get(), s->hwnd, &desc, nullptr, nullptr,
                                                   &s->swap))) {
            delete s;
            return nullptr;
        }
    }
    if (FAILED(g.device->CreateDeviceContext(D2D1_DEVICE_CONTEXT_OPTIONS_NONE, &s->ctx))) {
        delete s;
        return nullptr;
    }
    if (!bindTarget(s)) {
        delete s;
        return nullptr;
    }
    return s;
}

__declspec(dllexport) void gtg_surface_free(void* p) { delete (GpuSurface*)p; }

__declspec(dllexport) int gtg_resize(void* p, int w, int h) {
    GpuSurface* s = (GpuSurface*)p;
    if (!s || !s->swap || w <= 0 || h <= 0) return 1;
    if ((UINT)w == s->w && (UINT)h == s->h) return 0;
    s->ctx->SetTarget(nullptr);
    s->target.Reset();
    if (FAILED(s->swap->ResizeBuffers(0, (UINT)w, (UINT)h, DXGI_FORMAT_UNKNOWN, 0))) return 2;
    s->w = (UINT)w;
    s->h = (UINT)h;
    return bindTarget(s) ? 0 : 3;
}

__declspec(dllexport) int gtg_present(void* p) {
    GpuSurface* s = (GpuSurface*)p;
    if (!s || !s->swap) return 1;
    HRESULT hr = s->swap->Present(1, 0);
    if (hr == DXGI_ERROR_DEVICE_REMOVED || hr == DXGI_ERROR_DEVICE_RESET) return 2;
    return SUCCEEDED(hr) ? 0 : 1;
}

// ---- canvas -------------------------------------------------------------

__declspec(dllexport) void* gtg_canvas_new(void* surf, unsigned clearArgb) {
    GpuSurface* s = (GpuSurface*)surf;
    if (!s || !s->ctx) return nullptr;
    GpuCanvas* c = new GpuCanvas();
    c->surf = s;
    s->ctx->BeginDraw();
    s->ctx->Clear(toColor(clearArgb));
    s->ctx->SetAntialiasMode(D2D1_ANTIALIAS_MODE_PER_PRIMITIVE);
    s->ctx->SetTextAntialiasMode(D2D1_TEXT_ANTIALIAS_MODE_GRAYSCALE);
    s->ctx->SetPrimitiveBlend(D2D1_PRIMITIVE_BLEND_SOURCE_OVER);
    return c;
}

__declspec(dllexport) void gtg_canvas_free(void* p) {
    GpuCanvas* c = (GpuCanvas*)p;
    if (!c) return;
    if (c->surf && c->surf->ctx) {
        while (c->clipN > 0) {
            c->clipN--;
            c->surf->ctx->PopAxisAlignedClip();
        }
        c->surf->ctx->EndDraw();
    }
    delete c;
}

__declspec(dllexport) void gtg_fill_rect(void* p, int x, int y, int w, int h, unsigned argb) {
    GpuCanvas* c = (GpuCanvas*)p;
    if (!c || w <= 0 || h <= 0) return;
    ID2D1SolidColorBrush* b = getBrush(c, argb);
    if (!b) return;
    c->surf->ctx->FillRectangle(D2D1::RectF((FLOAT)x, (FLOAT)y, (FLOAT)(x + w), (FLOAT)(y + h)), b);
}

__declspec(dllexport) void gtg_fill_round(void* p, int x, int y, int w, int h, int rad, unsigned argb) {
    GpuCanvas* c = (GpuCanvas*)p;
    if (!c || w <= 0 || h <= 0) return;
    ID2D1SolidColorBrush* b = getBrush(c, argb);
    if (!b) return;
    if (rad < 1) {
        c->surf->ctx->FillRectangle(D2D1::RectF((FLOAT)x, (FLOAT)y, (FLOAT)(x + w), (FLOAT)(y + h)), b);
        return;
    }
    ComPtr<ID2D1PathGeometry> geo = roundPath((FLOAT)x, (FLOAT)y, (FLOAT)w, (FLOAT)h, (FLOAT)rad);
    if (geo) c->surf->ctx->FillGeometry(geo.Get(), b);
}

__declspec(dllexport) void gtg_stroke_round(void* p, int x, int y, int w, int h, int rad,
                                            int width10, unsigned argb) {
    GpuCanvas* c = (GpuCanvas*)p;
    if (!c || w <= 0 || h <= 0) return;
    ID2D1SolidColorBrush* b = getBrush(c, argb);
    if (!b) return;
    float width = width10 / 10.0f;
    if (width < 0.5f) width = 0.5f;
    float inset = width10 / 20.0f;
    ComPtr<ID2D1PathGeometry> geo =
        roundPath(x + inset, y + inset, w - 2 * inset, h - 2 * inset, (FLOAT)rad - inset);
    if (geo) c->surf->ctx->DrawGeometry(geo.Get(), b, width);
}

__declspec(dllexport) void gtg_line(void* p, int x1, int y1, int x2, int y2, int width10,
                                    unsigned argb) {
    GpuCanvas* c = (GpuCanvas*)p;
    if (!c) return;
    ID2D1SolidColorBrush* b = getBrush(c, argb);
    if (!b) return;
    float width = width10 / 10.0f;
    if (width < 0.5f) width = 0.5f;
    c->surf->ctx->DrawLine(D2D1::Point2F((FLOAT)x1, (FLOAT)y1), D2D1::Point2F((FLOAT)x2, (FLOAT)y2),
                           b, width);
}

__declspec(dllexport) void gtg_circle(void* p, int cx, int cy, int r, unsigned argb) {
    GpuCanvas* c = (GpuCanvas*)p;
    if (!c || r <= 0) return;
    ID2D1SolidColorBrush* b = getBrush(c, argb);
    if (!b) return;
    c->surf->ctx->FillEllipse(D2D1::Ellipse(D2D1::Point2F((FLOAT)cx, (FLOAT)cy), (FLOAT)r, (FLOAT)r), b);
}

__declspec(dllexport) void gtg_stroke_circle(void* p, int cx, int cy, int r, int width10,
                                             unsigned argb) {
    GpuCanvas* c = (GpuCanvas*)p;
    if (!c || r <= 0) return;
    ID2D1SolidColorBrush* b = getBrush(c, argb);
    if (!b) return;
    float width = width10 / 10.0f;
    if (width < 0.5f) width = 0.5f;
    c->surf->ctx->DrawEllipse(D2D1::Ellipse(D2D1::Point2F((FLOAT)cx, (FLOAT)cy), (FLOAT)r, (FLOAT)r),
                              b, width);
}

__declspec(dllexport) void gtg_text(void* p, int x, int y, int w, int h, const WCHAR* s, int size,
                                    unsigned argb, int align, int bold, int wrap) {
    GpuCanvas* c = (GpuCanvas*)p;
    if (!c || !s || w <= 0) return;
    IDWriteTextFormat* f = getFormat(size, bold);
    ID2D1SolidColorBrush* b = getBrush(c, argb);
    if (!f || !b) return;
    f->SetTextAlignment(align == 1 ? DWRITE_TEXT_ALIGNMENT_CENTER
                                   : (align == 2 ? DWRITE_TEXT_ALIGNMENT_TRAILING
                                                 : DWRITE_TEXT_ALIGNMENT_LEADING));
    f->SetWordWrapping(wrap ? DWRITE_WORD_WRAPPING_WRAP : DWRITE_WORD_WRAPPING_NO_WRAP);
    c->surf->ctx->DrawText(s, (UINT32)wcslen(s), f, D2D1::RectF((FLOAT)x, (FLOAT)y, (FLOAT)(x + w),
                                                               (FLOAT)(y + h)), b,
                           D2D1_DRAW_TEXT_OPTIONS_CLIP);
}

__declspec(dllexport) void gtg_measure(void* p, const WCHAR* s, int size, int bold, int* outW,
                                       int* outH) {
    if (outW) *outW = 0;
    if (outH) *outH = 0;
    if (!p || !s || !g.dwrite || !g.factory) return;
    IDWriteTextFormat* f = getFormat(size, bold);
    if (!f) return;
    f->SetWordWrapping(DWRITE_WORD_WRAPPING_NO_WRAP);
    ComPtr<IDWriteTextLayout> layout;
    if (FAILED(g.dwrite->CreateTextLayout(s, (UINT32)wcslen(s), f, 10000.0f, (FLOAT)size * 2.0f,
                                          &layout)))
        return;
    DWRITE_TEXT_METRICS m = {};
    if (FAILED(layout->GetMetrics(&m))) return;
    if (outW) *outW = (int)ceilf(m.widthIncludingTrailingWhitespace);
    if (outH) *outH = (int)ceilf(m.height);
}

__declspec(dllexport) void gtg_push_clip(void* p, int x, int y, int w, int h) {
    GpuCanvas* c = (GpuCanvas*)p;
    if (!c || c->clipN >= 32) return;
    D2D1_RECT_F r = D2D1::RectF((FLOAT)x, (FLOAT)y, (FLOAT)(x + w), (FLOAT)(y + h));
    c->clipRect[c->clipN++] = r;
    c->surf->ctx->PushAxisAlignedClip(r, D2D1_ANTIALIAS_MODE_ALIASED);
}

__declspec(dllexport) void gtg_pop_clip(void* p) {
    GpuCanvas* c = (GpuCanvas*)p;
    if (!c || c->clipN <= 0) return;
    c->clipN--;
    c->surf->ctx->PopAxisAlignedClip();
}

__declspec(dllexport) void gtg_set_view(void* p, int milli, int ox, int oy) {
    GpuCanvas* c = (GpuCanvas*)p;
    if (!c || milli <= 0) return;
    c->scale = milli / 1000.0f;
    c->offX = ox;
    c->offY = oy;
    c->surf->ctx->SetTransform(D2D1::Matrix3x2F::Scale(c->scale, c->scale) *
                               D2D1::Matrix3x2F::Translation((FLOAT)ox, (FLOAT)oy));
}

__declspec(dllexport) void gtg_set_scale(void* p, int milli) {
    GpuCanvas* c = (GpuCanvas*)p;
    if (!c || milli <= 0) return;
    c->scale = milli / 1000.0f;
    c->offX = 0;
    c->offY = 0;
    c->surf->ctx->SetTransform(D2D1::Matrix3x2F::Scale(c->scale, c->scale));
}

__declspec(dllexport) void gtg_glyph(void* p, const WCHAR* s, int x, int y, int w, int h, int size,
                                     unsigned argb) {
    GpuCanvas* c = (GpuCanvas*)p;
    if (!c || !s || w <= 0 || h <= 0 || g.iconName.empty()) return;
    ID2D1SolidColorBrush* b = getBrush(c, argb);
    if (!b) return;
    ComPtr<IDWriteTextFormat> f;
    if (FAILED(g.dwrite->CreateTextFormat(g.iconName.c_str(), nullptr, DWRITE_FONT_WEIGHT_NORMAL,
            DWRITE_FONT_STYLE_NORMAL, DWRITE_FONT_STRETCH_NORMAL, (FLOAT)size, L"", &f)))
        return;
    f->SetTextAlignment(DWRITE_TEXT_ALIGNMENT_CENTER);
    f->SetParagraphAlignment(DWRITE_PARAGRAPH_ALIGNMENT_CENTER);
    f->SetWordWrapping(DWRITE_WORD_WRAPPING_NO_WRAP);
    c->surf->ctx->DrawText(s, (UINT32)wcslen(s), f.Get(),
                           D2D1::RectF((FLOAT)x, (FLOAT)y, (FLOAT)(x + w), (FLOAT)(y + h)), b,
                           D2D1_DRAW_TEXT_OPTIONS_CLIP);
}

__declspec(dllexport) int gtg_set_font_family(const WCHAR* name) {
    if (!g.ok || !name) return 1;
    ComPtr<IDWriteFontCollection> coll;
    if (FAILED(g.dwrite->GetSystemFontCollection(&coll))) return 2;
    UINT32 idx = 0;
    BOOL exists = FALSE;
    if (FAILED(coll->FindFamilyName(name, &idx, &exists)) || !exists) return 2;
    g.familyName = name;
    g_texts.clear();
    return 0;
}

__declspec(dllexport) void gtg_image_size(const WCHAR* path, int* w, int* h) {
    if (w) *w = 0;
    if (h) *h = 0;
    if (!g.wic || !path) return;
    ComPtr<IWICBitmapDecoder> dec;
    if (FAILED(g.wic->CreateDecoderFromFilename(path, nullptr, GENERIC_READ,
            WICDecodeMetadataCacheOnLoad, &dec)))
        return;
    ComPtr<IWICBitmapFrameDecode> frame;
    if (FAILED(dec->GetFrame(0, &frame))) return;
    UINT iw = 0, ih = 0;
    frame->GetSize(&iw, &ih);
    if (w) *w = (int)iw;
    if (h) *h = (int)ih;
}

// gtg_fill_linear: two-stop gradient, vertical=1 runs top-to-bottom.
__declspec(dllexport) void gtg_fill_linear(void* p, int x, int y, int w, int h, unsigned c1,
                                           unsigned c2, int vertical) {
    GpuCanvas* c = (GpuCanvas*)p;
    if (!c || w <= 0 || h <= 0) return;
    ID2D1LinearGradientBrush* br = getGradient(c, c1, c2, vertical != 0);
    if (!br) return;
    // Map the unit square onto the target rect.
    D2D1_MATRIX_3X2_F saved;
    c->surf->ctx->GetTransform(&saved);
    D2D1_MATRIX_3X2_F place = D2D1::Matrix3x2F::Scale((FLOAT)w, (FLOAT)h) *
                             D2D1::Matrix3x2F::Translation((FLOAT)x, (FLOAT)y);
    c->surf->ctx->SetTransform(place * saved);
    c->surf->ctx->FillRectangle(D2D1::RectF(0, 0, 1, 1), br);
    c->surf->ctx->SetTransform(saved);
}

// gtg_fill_radial: centre-out gradient filling an ellipse inscribed in the rect.
__declspec(dllexport) void gtg_fill_radial(void* p, int x, int y, int w, int h, unsigned center,
                                           unsigned edge) {
    GpuCanvas* c = (GpuCanvas*)p;
    if (!c || w <= 0 || h <= 0) return;
    ComPtr<ID2D1GradientStopCollection> stops;
    D2D1_GRADIENT_STOP gs[2];
    gs[0].position = 0.0f;
    gs[0].color = toColor(center);
    gs[1].position = 1.0f;
    gs[1].color = toColor(edge);
    if (FAILED(c->surf->ctx->CreateGradientStopCollection(gs, 2, &stops))) return;
    ComPtr<ID2D1RadialGradientBrush> br;
    if (FAILED(c->surf->ctx->CreateRadialGradientBrush(
            D2D1::RadialGradientBrushProperties(
                D2D1::Point2F(x + w / 2.0f, y + h / 2.0f), D2D1::Point2F(0, 0),
                w / 2.0f, h / 2.0f),
            stops.Get(), &br)))
        return;
    c->surf->ctx->FillEllipse(
        D2D1::Ellipse(D2D1::Point2F(x + w / 2.0f, y + h / 2.0f), w / 2.0f, h / 2.0f), br.Get());
}

__declspec(dllexport) void gtg_shadow(void* p, int x, int y, int w, int h, int rad, int blur,
                                     unsigned argb) {
    GpuCanvas* c = (GpuCanvas*)p;
    if (!c || w <= 0 || h <= 0 || blur <= 0) return;
    if (ID2D1Bitmap* b = bakedShadow(c, w, h, blur, argb)) {
        int inset = blur * 2;
        // One pixel low so the light reads as coming from above.
        D2D1_RECT_F dst = D2D1::RectF((FLOAT)(x - inset), (FLOAT)(y - inset + 1),
                                      (FLOAT)(x + w + inset), (FLOAT)(y + h + inset + 1));
        c->surf->ctx->DrawBitmap(b, &dst, 1.0f, D2D1_BITMAP_INTERPOLATION_MODE_LINEAR);
        return;
    }
    if (!g_shadow.bmp || g_shadow.blur != blur) {
        if (!buildShadow(c, blur, argb)) return;
    }
    // The shadow sits one pixel low so the light reads as coming from above.
    D2D1_RECT_F r = D2D1::RectF((FLOAT)(x - g_shadow.inset), (FLOAT)(y - g_shadow.inset + 1),
                                (FLOAT)(x + w + g_shadow.inset),
                                (FLOAT)(y + h + g_shadow.inset + 1));
    drawNineSlice(c, r, 1.0f);
}

__declspec(dllexport) int gtg_draw_image(void* p, const WCHAR* path, int x, int y, int w, int h,
                                         int alpha) {
    GpuCanvas* c = (GpuCanvas*)p;
    if (!c || w <= 0 || h <= 0) return 0;
    int iw = 0, ih = 0;
    ID2D1Bitmap* bmp = getImage(c, path, &iw, &ih);
    if (!bmp) return 0;
    float op = alpha < 0 ? 1.0f : (alpha > 255 ? 1.0f : alpha / 255.0f);
    c->surf->ctx->DrawBitmap(bmp,
                             D2D1::RectF((FLOAT)x, (FLOAT)y, (FLOAT)(x + w), (FLOAT)(y + h)),
                             op, D2D1_BITMAP_INTERPOLATION_MODE_LINEAR);
    return 1;
}

} // extern "C"
