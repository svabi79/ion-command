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

    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double GlobeRadius = 1000.0;
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") int32 SegmentsPerArc = 12;
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") int32 MaxVisibleArcs = 180;
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double LifetimeSeconds = 12.0;
    // Cylinder scale factors; the base mesh is 100 units wide, so 0.05 renders
    // a 5-unit beam that stays visible from the showcase camera distance.
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double ArcThickness = 0.05;
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double SelectedArcThickness = 0.10;

protected:
    virtual int32 ResolvePaletteIndex(const FGeoMessageEnvelope& Message) const;
    virtual FLinearColor ResolvePaletteColor(int32 PaletteIndex) const;
    virtual FGeoLayerManifest CreateLayerManifest() const;

private:
    void OnMessageAccepted(const FGeoMessageEnvelope& Message);
    void BuildEditorPreview();
    void AddArcInstances(const FRenderedGeoArc& Arc);
    void AddArcInstancesTo(UInstancedStaticMeshComponent* Instances, const FGeoMessageEnvelope& Message, double Thickness);
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
    bool bSelectionDimmed = false;
    double LastExpiryCheck = 0.0;
};
