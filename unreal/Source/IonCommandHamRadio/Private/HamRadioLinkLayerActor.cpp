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
