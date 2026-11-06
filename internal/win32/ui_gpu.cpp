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
