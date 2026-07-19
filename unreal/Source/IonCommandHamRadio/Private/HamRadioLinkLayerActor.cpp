#include "HamRadioLinkLayerActor.h"

#include "HamBandVisualConfig.h"

AHamRadioLinkLayerActor::AHamRadioLinkLayerActor()
{
    BandVisualConfig = LoadObject<UHamBandVisualConfig>(nullptr, TEXT("/Game/ION/Data/DA_BandVisualConfig.DA_BandVisualConfig"));
}

bool AHamRadioLinkLayerActor::Supports(const FGeoMessageEnvelope& Message) const
{
    return Message.SemanticType == TEXT("radio.reception") && Super::Supports(Message);
}

int32 AHamRadioLinkLayerActor::ResolvePaletteIndex(const FGeoMessageEnvelope& Message) const
{
    if (BandVisualConfig) return BandVisualConfig->ResolvePaletteIndex(Message.Properties.FindRef(TEXT("band")));
    const UHamBandVisualConfig* Defaults = GetDefault<UHamBandVisualConfig>();
    return Defaults->ResolvePaletteIndex(Message.Properties.FindRef(TEXT("band")));
}

FLinearColor AHamRadioLinkLayerActor::ResolvePaletteColor(int32 PaletteIndex) const
{
    const UHamBandVisualConfig* Config = BandVisualConfig ? BandVisualConfig.Get() : GetDefault<UHamBandVisualConfig>();
    for (const FHamBandVisualDefinition& Band : Config->Bands) if (Band.PaletteIndex == PaletteIndex) return Band.Color;
    return Super::ResolvePaletteColor(PaletteIndex);
}

FString AHamRadioLinkLayerActor::ResolvePaletteLabel(int32 PaletteIndex) const
{
    // Palettes can carry several bands (15m/12m share one); join their ids.
    const UHamBandVisualConfig* Config = BandVisualConfig ? BandVisualConfig.Get() : GetDefault<UHamBandVisualConfig>();
    FString Label;
    for (const FHamBandVisualDefinition& Band : Config->Bands)
    {
        if (Band.PaletteIndex != PaletteIndex) continue;
        if (!Label.IsEmpty()) Label += TEXT("/");
        Label += Band.BandId.ToUpper();
    }
    return Label.IsEmpty() ? TEXT("OTHER") : Label;
}

FString AHamRadioLinkLayerActor::ResolveTrafficPanelTitle() const
{
    return TEXT("BAND ACTIVITY");
}

FGeoLayerManifest AHamRadioLinkLayerActor::CreateLayerManifest() const
{
    FGeoLayerManifest Manifest;
    Manifest.LayerId = TEXT("hamradio.live-links");
    Manifest.DisplayName = TEXT("Live Radio Links");
    Manifest.Domain = TEXT("hamradio");
    Manifest.AcceptedSemanticTypes = {TEXT("radio.reception")};
    Manifest.GeometryTypes = {EGeoGeometryType::GreatCircle, EGeoGeometryType::Arc};
    Manifest.bSupportsAggregation = true;
    return Manifest;
}
