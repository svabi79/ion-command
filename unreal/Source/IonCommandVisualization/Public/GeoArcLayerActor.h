#pragma once

#include "CoreMinimal.h"
#include "GameFramework/Actor.h"
#include "GeoLayerTypes.h"
#include "GeoArcLayerActor.generated.h"

class UGeoDataSubsystem;
class UInstancedStaticMeshComponent;
class UMaterialInstanceDynamic;

USTRUCT()
struct FRenderedGeoArc
{
    GENERATED_BODY()
    FGeoMessageEnvelope Message;
    FDateTime AddedUtc;
    double SpawnTimeSeconds = 0.0;
    int32 PaletteIndex = 0;
};

UCLASS()
class IONCOMMANDVISUALIZATION_API AGeoArcLayerActor : public AActor, public IGeoRenderAdapter
{
    GENERATED_BODY()

public:
    AGeoArcLayerActor();
    virtual void OnConstruction(const FTransform& Transform) override;
    virtual void PostRegisterAllComponents() override;
    virtual void BeginPlay() override;
    virtual void EndPlay(const EEndPlayReason::Type EndPlayReason) override;
    virtual void Tick(float DeltaSeconds) override;

    virtual bool Supports(const FGeoMessageEnvelope& Message) const override;
    virtual void Submit(const FGeoMessageEnvelope& Message) override;
    virtual void Reset() override;

    // Performs a bounded CPU pick only when the operator clicks. This avoids
    // persistent collision bodies for the thousands of instanced arc segments.
    bool FindClosestMessageToRay(const FVector& RayOrigin, const FVector& RayDirection, double RayLength, double MaxDistance, FGeoMessageEnvelope& OutMessage) const;

    // Restricts the layer to relationships touching one of the given entity
    // ids (empty clears the filter) and resubmits the active window.
    void SetEntityFilter(const TArray<FString>& EntityIds);
    bool HasEntityFilter() const { return EntityFilter.Num() > 0; }

    // Shows a single palette (band) exclusively; pressing the same preset
    // again or passing INDEX_NONE restores all bands. Pure visibility, no
    // data churn.
    void SetBandFocus(int32 PaletteIndex);
    int32 GetBandFocus() const { return BandFocusIndex; }

    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double GlobeRadius = 1000.0;
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") int32 SegmentsPerArc = 16;
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") int32 MaxVisibleArcs = 10000;
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double LifetimeSeconds = 18.0;
    // Cylinder scale factors; the base mesh is 100 units wide. Thin beams plus
    // bloom read as light, thick ones read as plastic tubes.
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double ArcThickness = 0.02;
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double SelectedArcThickness = 0.055;

protected:
    virtual int32 ResolvePaletteIndex(const FGeoMessageEnvelope& Message) const;
    virtual FLinearColor ResolvePaletteColor(int32 PaletteIndex) const;
    virtual FGeoLayerManifest CreateLayerManifest() const;

private:
    void OnMessageAccepted(const FGeoMessageEnvelope& Message);
    void BuildEditorPreview();
    void AddArcInstances(const FRenderedGeoArc& Arc);
    void AddArcInstancesTo(UInstancedStaticMeshComponent* Instances, const FGeoMessageEnvelope& Message, double Thickness);
    void AppendArcTransforms(TArray<FTransform>& Out, const FGeoMessageEnvelope& Message, double Thickness) const;
    FVector CalculateArcPoint(const FGeoMessageEnvelope& Message, double Alpha) const;
    void RebuildInstances();
    void RefreshSelectionHighlight();
    void ApplySelectionDimming(bool bDim);
    void OnLayerVisibilityChanged(const FString& LayerId, bool bVisible);
    UPROPERTY(VisibleAnywhere) TObjectPtr<USceneComponent> SceneRoot;
    UPROPERTY(VisibleAnywhere) TArray<TObjectPtr<UInstancedStaticMeshComponent>> PaletteMeshes;
    UPROPERTY(VisibleAnywhere) TObjectPtr<UInstancedStaticMeshComponent> SelectionMesh;
    UPROPERTY(VisibleAnywhere) TObjectPtr<UInstancedStaticMeshComponent> EndpointMesh;
    UPROPERTY(Transient) TArray<TObjectPtr<UMaterialInstanceDynamic>> PaletteMaterials;
    UPROPERTY(Transient) TArray<FRenderedGeoArc> ActiveArcs;
    TWeakObjectPtr<UGeoDataSubsystem> DataSubsystem;
    FString HighlightedMessageId;
    TArray<float> PaletteBaseIntensities;
    TArray<FString> EntityFilter;
    int32 BandFocusIndex = INDEX_NONE;
    bool bSelectionDimmed = false;
    bool bNeedsRebuild = false;
    double LastExpiryCheck = 0.0;
};
