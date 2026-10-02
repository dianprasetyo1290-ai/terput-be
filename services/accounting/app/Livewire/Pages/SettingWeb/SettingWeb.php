<?php

namespace App\Livewire\Pages\SettingWeb;

use App\Jobs\CompressImage;
use App\Models\SettingWeb as ModelSettingWeb;
use Illuminate\Support\Facades\Storage;
use Livewire\Component;
use Livewire\WithFileUploads;

class SettingWeb extends Component
{
    use WithFileUploads;

    public $setting = [
        'title' => null,
        'slogan' => null,
        'site_description' => null,
        'meta_description' => null,
        'meta_keyword' => null,
        'email' => null,
        'address' => null,
        'phone' => null,
        'logo' => null,
        'logo_name' => null,
        'favicon' => null,
        'favicon_name' => null,
        'logo_sidebar' => null,
    ];
    public function getImageUrlLogo()
    {
        if ($this->setting['logo'] && method_exists($this->setting['logo'], 'temporaryUrl') && $this->setting['logo']->temporaryUrl()) {
            return $this->setting['logo']->temporaryUrl();
        } elseif ($this->setting['logo']) {
            return route('helper.show-picture', ['path' => $this->setting['logo']]);
        } else {
            return asset('limitless4.0/assets/images/placeholders/placeholder.jpg');
        }
    }

    public function getImageUrlLogoSideBar()
    {
        if ($this->setting['logo_sidebar'] && method_exists($this->setting['logo_sidebar'], 'temporaryUrl') && $this->setting['logo_sidebar']->temporaryUrl()) {
            return $this->setting['logo_sidebar']->temporaryUrl();
        } elseif ($this->setting['logo_sidebar']) {
            return route('helper.show-picture', ['path' => $this->setting['logo_sidebar']]);
        } else {
            return asset('limitless4.0/assets/images/placeholders/placeholder.jpg');
        }
    }

    public function getImageUrlFavicon()
    {
        if ($this->setting['favicon'] && method_exists($this->setting['favicon'], 'temporaryUrl') && $this->setting['favicon']->temporaryUrl()) {
            return $this->setting['favicon']->temporaryUrl();
        } elseif ($this->setting['favicon']) {
            return route('helper.show-picture', ['path' => $this->setting['favicon']]);
        } else {
            return asset('limitless4.0/assets/images/placeholders/placeholder.jpg');
        }
    }

    public function save()
    {
        $settingModel = ModelSettingWeb::find(1);

        $this->setting['logo'] = $this->handleImageUpload(
            $this->setting['logo'],
            $settingModel->logo
        );

        $this->setting['logo_sidebar'] = $this->handleImageUpload(
            $this->setting['logo_sidebar'],
            $settingModel->logo_sidebar
        );

        $this->setting['favicon'] = $this->handleImageUpload(
            $this->setting['favicon'],
            $settingModel->favicon
        );

        $settingModel->update($this->setting);
        $this->setting = $settingModel->toArray();

        $this->js(<<<'JS'
        swalInit.fire({
            title: 'Change Setting Successful!',
            text: 'Great!',
            icon: 'success'
        }).then(function(result) {
            if(result.isConfirmed) {
                $wire.confirm()
            }
        });
     JS);
    }

    public function confirm()
    {
        return redirect()->route('settingwebsite');
    }

    public function mount()
    {
        $this->setting = ModelSettingWeb::find(1)->toArray();
    }
    private function handleImageUpload($newImageFile, $oldImagePathFromDb): ?string
    {
        if ($newImageFile instanceof \Livewire\Features\SupportFileUploads\TemporaryUploadedFile) {
            // Delete old image if it exists
            if ($oldImagePathFromDb && Storage::exists($oldImagePathFromDb)) {
                Storage::delete($oldImagePathFromDb);
            }

            // Store new image
            $path = $newImageFile->store('upload/setting', 'public');
            CompressImage::dispatch($path); // Dispatch compression job

            return $path;
        } elseif ($oldImagePathFromDb) {
            // No new image uploaded, retain existing image path
            return $oldImagePathFromDb;
        }

        return null; // No image at all
    }
    public function render()
    {
        return view('livewire.pages.setting-web.setting-web');
    }
}
