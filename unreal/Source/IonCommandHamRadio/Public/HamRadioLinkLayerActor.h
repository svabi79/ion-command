#pragma once

#include "CoreMinimal.h"
#include "GeoArcLayerActor.h"
#include "HamRadioLinkLayerActor.generated.h"

class UHamBandVisualConfig;

UCLASS()
class IONCOMMANDHAMRADIO_API AHamRadioLinkLayerActor final : public AGeoArcLayerActor
{
    GENERATED_BODY()

public:
    AHamRadioLinkLayerActor();
    virtual bool Supports(const FGeoMessageEnvelope& Message) const override;

protected:
    virtual int32 ResolvePaletteIndex(const FGeoMessageEnvelope& Message) const override;
    virtual FLinearColor ResolvePaletteColor(int32 PaletteIndex) const override;
    virtual FString ResolvePaletteLabel(int32 PaletteIndex) const override;
    virtual FString ResolveTrafficPanelTitle() const override;
    virtual FGeoLayerManifest CreateLayerManifest() const override;

private:
    UPROPERTY(EditDefaultsOnly, Category="ION COMMAND|Ham Radio") TObjectPtr<UHamBandVisualConfig> BandVisualConfig;
};
