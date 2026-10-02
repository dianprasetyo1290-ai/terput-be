<?php

namespace App\Jobs;

use Illuminate\Contracts\Queue\ShouldQueue;
use Illuminate\Foundation\Bus\Dispatchable;
use Illuminate\Foundation\Queue\Queueable;
use Illuminate\Queue\InteractsWithQueue;
use Illuminate\Queue\SerializesModels;
use Illuminate\Support\Facades\Storage;
use Intervention\Image\Laravel\Facades\Image;

class CompressImage implements ShouldQueue
{
    use Dispatchable, InteractsWithQueue, Queueable, SerializesModels;

    public $image;

    public function __construct($image)
    {
        $this->image = $image;
    }

    /**
     * Execute the job.
     */
    public function handle(): void
    {
        $filePath = Storage::path($this->image);
        $image = Image::read($filePath);
        [$width, $height] = getimagesize($filePath);
        $sizeInBytes = Storage::size($this->image);
        if ($sizeInBytes > 6144) {
            $a = 2;
            $image->resize($width / $a, $height / $a);
        } else {
            $a = 1;
            $image->resize($width / $a, $height / $a);
        }
        $image->save($filePath);
    }
}
